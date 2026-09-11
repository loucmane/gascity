//go:build linux

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

const (
	domain        = "gc.synthetic-metadata-channel.v1"
	receiptSchema = "gc.synthetic-committed-pair.v1"
	maxFrame      = 16384
)

type binding struct {
	Protocol    string `json:"protocol"`
	Transaction string `json:"transaction"`
	Nonce       string `json:"nonce"`
	Request     string `json:"request"`
	Channel     string `json:"channel"`
	Epoch       string `json:"epoch"`
	Deadline    int64  `json:"deadline"`
}

type frame struct {
	Binding  binding `json:"binding"`
	Kind     string  `json:"kind"`
	Sequence int     `json:"sequence"`
	Digest   string  `json:"digest"`
	Evidence string  `json:"evidence,omitempty"`
}

type leaseRequest struct {
	Transaction string `json:"transaction"`
	Nonce       string `json:"nonce"`
	Request     string `json:"request"`
	Deadline    int64  `json:"deadline"`
}

type leaseSlot struct {
	Instance string
	Active   bool
	Request  leaseRequest
}

func (slot *leaseSlot) begin(request leaseRequest, now int64) error {
	if request.Transaction == "" || request.Nonce == "" || request.Request == "" || request.Deadline <= now || request.Deadline-now > 30_000_000_000 {
		return fmt.Errorf("invalid lease request")
	}
	if slot.Active && now < slot.Request.Deadline {
		return fmt.Errorf("lease occupied; renewal refused")
	}
	slot.Active = true
	slot.Request = request
	return nil
}

func (slot leaseSlot) check(request leaseRequest, epoch string, now int64) error {
	if !slot.Active || slot.Request != request || slot.Instance != epoch || now >= request.Deadline {
		return fmt.Errorf("lease missing, stale or expired")
	}
	return nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func checkFrame(value frame, expected binding, kind string, sequence int, now int64) error {
	if value.Binding != expected || value.Kind != kind || value.Sequence != sequence || now >= expected.Deadline {
		return fmt.Errorf("closed channel binding/order/deadline refused")
	}
	return nil
}

func rejectDuplicates(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("invalid delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate key")
			}
			seen[name] = true
		}
		if err := rejectDuplicates(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func strictJSON(data []byte, target any) error {
	if len(data) > maxFrame {
		return fmt.Errorf("oversized message")
	}
	if err := rejectDuplicates(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing message")
	}
	return nil
}

type receipt struct {
	Schema      string `json:"schema"`
	Transaction string `json:"transaction"`
	Epoch       string `json:"epoch"`
	Manifest    string `json:"manifest"`
}

func receiptBytes(transaction, epoch, manifest string) []byte {
	data, _ := json.Marshal(receipt{receiptSchema, transaction, epoch, manifest})
	return data
}

func committedPair(first, manifest, second []byte) error {
	var value receipt
	if !bytes.Equal(first, second) {
		return fmt.Errorf("receipt changed during read")
	}
	if err := strictJSON(first, &value); err != nil {
		return err
	}
	if value.Schema != receiptSchema || value.Transaction == "" || value.Epoch == "" || value.Manifest != digest(manifest) {
		return fmt.Errorf("pending/mixed/unsupported pair")
	}
	return nil
}

func disposition(committed bool, err error) string {
	if err == nil && committed {
		return "PASS"
	}
	if committed {
		return "committed_evidence_incomplete"
	}
	return "aborted_preserved"
}

type transactionOps struct{ Step func(string) error }

func publish(ops transactionOps) (bool, error) {
	committed := false
	for _, step := range []string{"stage", "manifest", "grant", "receipt", "sync", "final"} {
		if err := ops.Step(step); err != nil {
			return committed, err
		}
		if step == "receipt" {
			committed = true
		}
	}
	return committed, nil
}
