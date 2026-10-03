package main

import (
	"encoding/json"
	"errors"
	"regexp"
)

type commitReceipt struct {
	ID          string                       `json:"id"`
	Fingerprint string                       `json:"fingerprint"`
	Revision    string                       `json:"revision"`
	Records     map[string][]json.RawMessage `json:"records,omitempty"`
}

var commitIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)

func identifyCommit(ids []string, method, revision string, payload any) (commitReceipt, error) {
	if len(ids) == 0 || ids[0] == "" {
		return commitReceipt{}, nil
	}
	if !commitIDPattern.MatchString(ids[0]) {
		return commitReceipt{}, errors.New("提交编号无效")
	}
	raw, err := json.Marshal([]any{method, revision, payload})
	if err != nil {
		return commitReceipt{}, err
	}
	return commitReceipt{ID: ids[0], Fingerprint: hashContent(raw)}, nil
}

func (s *diskStore) replayCommit(receipts []commitReceipt, request commitReceipt) (map[string]any, error) {
	if request.ID == "" {
		return nil, nil
	}
	for _, receipt := range receipts {
		if receipt.ID != request.ID {
			continue
		}
		if receipt.Fingerprint != request.Fingerprint {
			return nil, errDataConflict
		}
		// Return this operation's revision, never adopt an intervening writer's
		// revision. A subsequent edit must still pass ordinary compare-and-swap.
		return map[string]any{"saved": true, "revision": receipt.Revision, "replayed": true, "storage": s.statusLocked(), "records": receipt.Records}, nil
	}
	return nil, nil
}

func appendCommit(receipts []commitReceipt, request commitReceipt, revision string) []commitReceipt {
	if request.ID == "" {
		return receipts
	}
	request.Revision = revision
	result := append(append([]commitReceipt(nil), receipts...), request)
	if len(result) > 64 {
		result = result[len(result)-64:]
	}
	return result
}
