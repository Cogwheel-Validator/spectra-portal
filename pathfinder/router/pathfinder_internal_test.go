package router

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
	"github.com/zeebo/assert"
)

// countingBroker implements brokers.BrokerClient and fails the first failUntil calls.
type countingBroker struct {
	attempts  int
	failUntil int
}

func (c *countingBroker) QuerySwap(tokenInDenom, tokenInAmount, tokenOutDenom string, singleRoute *bool) (*brokers.SwapResult, error) {
	c.attempts++
	if c.attempts <= c.failUntil {
		return nil, errors.New("sqs unavailable")
	}
	return &brokers.SwapResult{
		AmountIn:  tokenInAmount,
		AmountOut: "990000",
	}, nil
}

func (c *countingBroker) GetBrokerType() string                                 { return "counting-broker" }
func (c *countingBroker) GetMemoBuilder() ibcmemo.MemoBuilder                   { return nil }
func (c *countingBroker) GetSmartContractBuilder() brokers.SmartContractBuilder { return nil }
func (c *countingBroker) Close()                                                {}

func TestQueryBrokerWithRetry_EventualSuccess(t *testing.T) {
	s := &Pathfinder{maxRetries: 3, retryDelay: time.Millisecond}
	client := &countingBroker{failUntil: 2}

	result, err := s.queryBrokerWithRetry(client, "1000000", "uatom", "uosmo", nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result.AmountOut, "990000")
	// Two failures, then success on the third attempt
	assert.Equal(t, client.attempts, 3)
}

func TestQueryBrokerWithRetry_Exhausted(t *testing.T) {
	s := &Pathfinder{maxRetries: 2, retryDelay: time.Millisecond}
	client := &countingBroker{failUntil: 100}

	result, err := s.queryBrokerWithRetry(client, "1000000", "uatom", "uosmo", nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	// Initial attempt + maxRetries retries
	assert.Equal(t, client.attempts, 3)
	if !strings.Contains(err.Error(), "counting-broker") {
		t.Errorf("error should identify the broker type, got: %v", err)
	}
	if !strings.Contains(err.Error(), "3 attempts") {
		t.Errorf("error should report the attempt count, got: %v", err)
	}
}

func TestCheckPFMSupport(t *testing.T) {
	ri := NewRouteIndex()
	ri.pfmChains["chain-b"] = true
	ri.pfmChains["chain-d"] = true
	s := &Pathfinder{routeIndex: ri}

	// Paths with no intermediate chains never need PFM
	assert.False(t, s.checkPFMSupport([]string{"chain-a"}))
	assert.False(t, s.checkPFMSupport([]string{"chain-a", "chain-b"}))

	// Only the intermediate chains need PFM, not the endpoints
	assert.True(t, s.checkPFMSupport([]string{"chain-a", "chain-b", "chain-c"}))
	assert.True(t, s.checkPFMSupport([]string{"chain-a", "chain-b", "chain-d", "chain-c"}))

	// A single non-PFM intermediate breaks support
	assert.False(t, s.checkPFMSupport([]string{"chain-a", "chain-x", "chain-c"}))
	assert.False(t, s.checkPFMSupport([]string{"chain-a", "chain-b", "chain-x", "chain-c"}))
}

func TestGeneratePFMMemo(t *testing.T) {
	s := &Pathfinder{}

	legs := []*models.IBCLeg{
		{FromChain: "a", ToChain: "b", Port: "transfer", Channel: "channel-1"},
		{FromChain: "b", ToChain: "c", Port: "transfer", Channel: "channel-2"},
		{FromChain: "c", ToChain: "d", Port: "transfer", Channel: "channel-3"},
	}

	memo := s.generatePFMMemo(legs, "d1finalreceiver")
	assert.True(t, memo != "")

	// The memo describes forwarding from the second leg onward and must be valid,
	// correctly nested JSON.
	var parsed struct {
		Forward struct {
			Receiver string `json:"receiver"`
			Port     string `json:"port"`
			Channel  string `json:"channel"`
			Next     *struct {
				Forward struct {
					Receiver string `json:"receiver"`
					Channel  string `json:"channel"`
				} `json:"forward"`
			} `json:"next"`
		} `json:"forward"`
	}
	assert.NoError(t, json.Unmarshal([]byte(memo), &parsed))
	assert.Equal(t, parsed.Forward.Channel, "channel-2")
	assert.Equal(t, parsed.Forward.Port, "transfer")
	assert.NotNil(t, parsed.Forward.Next)
	assert.Equal(t, parsed.Forward.Next.Forward.Channel, "channel-3")
	assert.Equal(t, parsed.Forward.Next.Forward.Receiver, "d1finalreceiver")
}

func TestGeneratePFMMemo_EmptyLegs(t *testing.T) {
	s := &Pathfinder{}
	assert.Equal(t, s.generatePFMMemo(nil, "receiver"), "")
}

func TestRouteKey(t *testing.T) {
	assert.Equal(t, routeKey("chain-a", "chain-b", "uatom"), "chain-a->chain-b:uatom")
}

func TestBuildIndex_EmptyChains(t *testing.T) {
	ri := NewRouteIndex()
	assert.Error(t, ri.BuildIndex(nil))
	assert.Error(t, ri.BuildIndex([]PathfinderChain{}))
}
