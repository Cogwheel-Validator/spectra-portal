// Package cosmostx is a small, dependency-light codec for the Cosmos SDK
// transaction pieces the Pathfinder MCP server needs: building unsigned
// messages, SignDocs (direct and amino-JSON) and decoding transactions.
//
// It deliberately covers only the message types Spectra routes use
// (IBC MsgTransfer, wasm MsgExecuteContract and the Osmosis poolmanager swaps).
// Anything else is decoded generically as type_url + base64 bytes.
package cosmostx

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// encoder appends protobuf fields. Helpers skip zero values the way proto3
// does; msg always writes the field, which matches gogoproto's behaviour for
// non-nullable embedded messages (e.g. Coin, Height).
type encoder struct{ b []byte }

func (e *encoder) str(num protowire.Number, s string) {
	if s == "" {
		return
	}
	e.b = protowire.AppendTag(e.b, num, protowire.BytesType)
	e.b = protowire.AppendString(e.b, s)
}

func (e *encoder) bytes(num protowire.Number, v []byte) {
	if len(v) == 0 {
		return
	}
	e.b = protowire.AppendTag(e.b, num, protowire.BytesType)
	e.b = protowire.AppendBytes(e.b, v)
}

func (e *encoder) msg(num protowire.Number, v []byte) {
	e.b = protowire.AppendTag(e.b, num, protowire.BytesType)
	e.b = protowire.AppendBytes(e.b, v)
}

func (e *encoder) u64(num protowire.Number, v uint64) {
	if v == 0 {
		return
	}
	e.b = protowire.AppendTag(e.b, num, protowire.VarintType)
	e.b = protowire.AppendVarint(e.b, v)
}

// field is one decoded protobuf field. Only varint and length-delimited wire
// types are kept; anything else is skipped.
type field struct {
	num int32 // plain int so field-number switches aren't treated as enums
	typ protowire.Type
	u   uint64
	b   []byte
}

func (f field) str() string { return string(f.b) }

func parseFields(b []byte) ([]field, error) {
	var out []field
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("invalid protobuf tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, fmt.Errorf("invalid varint: %w", protowire.ParseError(n))
			}
			out = append(out, field{num: int32(num), typ: typ, u: v})
			b = b[n:]
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, fmt.Errorf("invalid length-delimited field: %w", protowire.ParseError(n))
			}
			out = append(out, field{num: int32(num), typ: typ, b: v})
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return nil, fmt.Errorf("invalid field: %w", protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return out, nil
}
