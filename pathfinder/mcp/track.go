package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const maxTrackHops = 6

// Packet statuses reported by track_transfer.
const (
	pktInFlight  = "in_flight"  // sent, not yet received on the destination
	pktExpired   = "expired"    // past its timeout and not received: a relayer will process the timeout and refund
	pktTimedOut  = "timed_out"  // timeout processed, funds refunded on the source
	pktReceived  = "received"   // received on the destination; acknowledgement not yet relayed back
	pktDelivered = "delivered"  // received and acknowledged
	pktFailed    = "failed_ack" // destination returned an error acknowledgement; funds are refunded
)

// PacketTrack is the state of one IBC packet and what happened downstream.
type PacketTrack struct {
	SourceChain string `json:"source_chain"`
	DestChain   string `json:"dest_chain,omitempty"`
	SourceTx    string `json:"source_tx_hash"`
	Sequence    string `json:"sequence"`
	SrcPort     string `json:"src_port"`
	SrcChannel  string `json:"src_channel"`
	DstPort     string `json:"dst_port,omitempty"`
	DstChannel  string `json:"dst_channel,omitempty"`
	Status      string `json:"status"`
	// TimeoutAt is the packet timeout (RFC 3339), when it has a timestamp timeout.
	TimeoutAt string `json:"timeout_at,omitempty"`
	RecvTx    string `json:"dest_tx_hash,omitempty"`
	AckError  string `json:"ack_error,omitempty"`
	Note      string `json:"note,omitempty"`
	// Forwarded are packets the destination chain sent on as a result of this
	// one (packet-forward-middleware or ibc-hooks swap-and-forward).
	Forwarded []*PacketTrack `json:"forwarded,omitempty"`
}

// TrackResult is the result of tracking a transaction.
type TrackResult struct {
	ChainID string `json:"chain_id"`
	TxHash  string `json:"tx_hash"`
	// Status is one of: not_found, tx_failed, no_ibc_packets, in_progress,
	// completed, failed, timed_out.
	Status  string         `json:"status"`
	Summary string         `json:"summary"`
	Tx      *txResult      `json:"tx,omitempty"`
	Packets []*PacketTrack `json:"packets,omitempty"`
}

type tracker struct {
	api *chainAPI
	now func() time.Time
}

func (t *tracker) track(ctx context.Context, chainID, hash string, follow bool) (*TrackResult, error) {
	tx, err := t.api.tx(ctx, chainID, hash)
	if err != nil {
		return nil, err
	}
	res := &TrackResult{ChainID: chainID, TxHash: tx.Hash, Tx: tx}
	if !tx.Found {
		res.Status = "not_found"
		res.Summary = "The transaction is not on-chain (yet). It may still be in the mempool, or the hash or chain_id is wrong. Retry in a few seconds."
		return res, nil
	}
	if tx.Code != 0 {
		res.Status = "tx_failed"
		res.Summary = fmt.Sprintf("The transaction failed on %s (code %d): %s. No IBC packets were sent.", chainID, tx.Code, tx.RawLog)
		return res, nil
	}
	pkts := sendPackets(tx.Events)
	if len(pkts) == 0 {
		res.Status = "no_ibc_packets"
		res.Summary = "The transaction succeeded but emitted no IBC send_packet event (for example a plain swap)."
		return res, nil
	}
	for _, p := range pkts {
		pt := &PacketTrack{
			SourceChain: chainID, SourceTx: tx.Hash,
			Sequence: p["packet_sequence"], SrcPort: p["packet_src_port"], SrcChannel: p["packet_src_channel"],
			DstPort: p["packet_dst_port"], DstChannel: p["packet_dst_channel"],
		}
		t.fill(ctx, pt, p, follow, 0)
		res.Packets = append(res.Packets, pt)
	}
	res.Status, res.Summary = overall(res.Packets)
	return res, nil
}

// fill resolves the status of pt and, when follow is set, the packets it caused.
func (t *tracker) fill(ctx context.Context, pt *PacketTrack, attrs map[string]string, follow bool, depth int) {
	if ts, err := strconv.ParseInt(attrs["packet_timeout_timestamp"], 10, 64); err == nil && ts > 0 {
		pt.TimeoutAt = time.Unix(0, ts).UTC().Format(time.RFC3339)
	}
	pt.DestChain = t.destChain(pt.SourceChain, pt.SrcPort, pt.SrcChannel)
	if pt.DestChain == "" {
		pt.Status = pktInFlight
		pt.Note = "Destination chain is not in the Pathfinder config, so delivery cannot be checked."
		return
	}
	committed, err := t.api.packetCommitted(ctx, pt.SourceChain, pt.SrcPort, pt.SrcChannel, pt.Sequence)
	if err != nil {
		pt.Status = pktInFlight
		pt.Note = "Could not read packet state from the source chain: " + err.Error()
		return
	}
	received, err := t.api.packetReceived(ctx, pt.DestChain, pt.DstPort, pt.DstChannel, pt.Sequence)
	if err != nil {
		pt.Status = pktInFlight
		pt.Note = "Could not read packet receipt from the destination chain: " + err.Error()
		return
	}
	switch {
	case !committed && !received:
		pt.Status = pktTimedOut
		pt.Note = "The packet timed out; the sent funds are refunded to the sender on " + pt.SourceChain + "."
		return
	case committed && !received:
		pt.Status = pktInFlight
		if ts, err := strconv.ParseInt(attrs["packet_timeout_timestamp"], 10, 64); err == nil && ts > 0 && t.now().UnixNano() > ts {
			pt.Status = pktExpired
			pt.Note = "The packet is past its timeout and was not received; a relayer should submit the timeout and refund the sender."
		}
		return
	case committed && received:
		pt.Status = pktReceived
	default:
		pt.Status = pktDelivered
	}
	if !follow || depth >= maxTrackHops {
		return
	}
	recv, err := t.findRecvTx(ctx, pt)
	if err != nil || recv == nil {
		return
	}
	pt.RecvTx = recv.Hash
	if ackErr := ackError(recv.Events, pt.Sequence, pt.SrcChannel); ackErr != "" {
		pt.Status = pktFailed
		pt.AckError = ackErr
		pt.Note = "The destination rejected the transfer; the funds are refunded on " + pt.SourceChain + "."
		return
	}
	for _, p := range sendPackets(recv.Events) {
		next := &PacketTrack{
			SourceChain: pt.DestChain, SourceTx: recv.Hash,
			Sequence: p["packet_sequence"], SrcPort: p["packet_src_port"], SrcChannel: p["packet_src_channel"],
			DstPort: p["packet_dst_port"], DstChannel: p["packet_dst_channel"],
		}
		t.fill(ctx, next, p, follow, depth+1)
		pt.Forwarded = append(pt.Forwarded, next)
	}
}

// destChain finds the chain a source channel leads to from the Pathfinder config.
func (t *tracker) destChain(src, port, channel string) string {
	ch, err := t.api.chain(src)
	if err != nil {
		return ""
	}
	for _, r := range ch.Routes {
		if r.ChannelId == channel && (r.PortId == port || r.PortId == "") {
			return r.ToChainId
		}
	}
	return ""
}

func (t *tracker) findRecvTx(ctx context.Context, pt *PacketTrack) (*txResult, error) {
	conds := []string{
		fmt.Sprintf("recv_packet.packet_sequence='%s'", pt.Sequence),
		fmt.Sprintf("recv_packet.packet_src_channel='%s'", pt.SrcChannel),
		fmt.Sprintf("recv_packet.packet_dst_channel='%s'", pt.DstChannel),
	}
	txs, err := t.api.searchTxs(ctx, pt.DestChain, conds, 1)
	if err != nil || len(txs) == 0 {
		return nil, err
	}
	return &txs[0], nil
}

// overall folds packet statuses into one status and a human summary.
func overall(roots []*PacketTrack) (status, summary string) {
	var leaves []*PacketTrack
	var walk func(p *PacketTrack)
	walk = func(p *PacketTrack) {
		if len(p.Forwarded) == 0 {
			leaves = append(leaves, p)
		}
		for _, f := range p.Forwarded {
			walk(f)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	var failed, timedOut, pending []string
	for _, l := range leaves {
		switch l.Status {
		case pktFailed:
			failed = append(failed, l.SourceChain+"->"+l.DestChain)
		case pktTimedOut:
			timedOut = append(timedOut, l.SourceChain+"->"+l.DestChain)
		case pktDelivered, pktReceived:
		default:
			pending = append(pending, l.SourceChain+"->"+l.DestChain)
		}
	}
	switch {
	case len(failed) > 0:
		return "failed", "An IBC transfer was rejected by the destination (" + strings.Join(failed, ", ") + "). Funds are refunded to the sender on the chain that sent that packet."
	case len(timedOut) > 0:
		return "timed_out", "An IBC packet timed out (" + strings.Join(timedOut, ", ") + "). Funds are refunded on the chain that sent it."
	case len(pending) > 0:
		return "in_progress", "Still in flight: " + strings.Join(pending, ", ") + ". Call track_transfer again in ~30-60 seconds."
	}
	return "completed", "All IBC packets were received by their destination chains."
}

// sendPackets extracts the attribute maps of every send_packet event.
func sendPackets(events []map[string]any) []map[string]string {
	var out []map[string]string
	for _, e := range events {
		if e["type"] != "send_packet" {
			continue
		}
		out = append(out, attrMap(e))
	}
	return out
}

func attrMap(e map[string]any) map[string]string {
	m := map[string]string{}
	attrs, _ := e["attributes"].([]any)
	for _, a := range attrs {
		kv, _ := a.(map[string]any)
		k, _ := kv["key"].(string)
		v, _ := kv["value"].(string)
		if k != "" {
			m[k] = v
		}
	}
	return m
}

// ackError returns the error text of the write_acknowledgement for a packet,
// or "" when it succeeded or no acknowledgement was written in this tx.
func ackError(events []map[string]any, seq, srcChannel string) string {
	for _, e := range events {
		if e["type"] != "write_acknowledgement" {
			continue
		}
		a := attrMap(e)
		if a["packet_sequence"] != seq || a["packet_src_channel"] != srcChannel {
			continue
		}
		ack := a["packet_ack"]
		if strings.Contains(ack, `"error"`) {
			return ack
		}
	}
	return ""
}
