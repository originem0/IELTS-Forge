package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type studyPatchBase struct {
	Metadata map[string]json.RawMessage            `json:"metadata"`
	Records  map[string]map[string]json.RawMessage `json:"records"`
	Orders   map[string][]string                   `json:"orders"`
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if len(a) > 0 && json.Unmarshal(a, &x) != nil {
		return false
	}
	if len(b) > 0 && json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// Check touched values even with a current revision: a client may not have
// loaded unrelated edits. Never advance its base from an unseen snapshot.
func validateStudyPatchBase(patch studyPatch, data json.RawMessage) error {
	var current map[string]json.RawMessage
	if json.Unmarshal(data, &current) != nil {
		return errDataConflict
	}
	for key := range patch.Metadata {
		expected, ok := patch.Base.Metadata[key]
		if !ok || (!sameJSON(expected, current[key]) && !sameJSON(patch.Metadata[key], current[key])) {
			return fmt.Errorf("%w：%s", errDataConflict, key)
		}
	}
	for collection, change := range patch.Collections {
		var entries []json.RawMessage
		if raw := current[collection]; len(raw) > 0 && json.Unmarshal(raw, &entries) != nil {
			return errDataConflict
		}
		records := map[string]json.RawMessage{}
		order := []string{}
		for _, raw := range entries {
			var id struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &id) != nil {
				return errDataConflict
			}
			records[id.ID] = raw
			order = append(order, id.ID)
		}
		for _, raw := range change.Upsert {
			var id struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &id) != nil {
				return errDataConflict
			}
			expected, ok := patch.Base.Records[collection][id.ID]
			if !ok || !sameJSON(expected, records[id.ID]) {
				return fmt.Errorf("%w：%s/%s", errDataConflict, collection, id.ID)
			}
		}
		if change.Order != nil {
			expected, ok := patch.Base.Orders[collection]
			if !ok || !reflect.DeepEqual(expected, order) {
				return fmt.Errorf("%w：%s 列表", errDataConflict, collection)
			}
		}
		if change.Order != nil {
			kept := map[string]bool{}
			for _, id := range change.Order {
				kept[id] = true
			}
			for id, raw := range records {
				if !kept[id] {
					expected, ok := patch.Base.Records[collection][id]
					if !ok || !sameJSON(expected, raw) {
						return fmt.Errorf("%w：%s/%s 删除前已改变", errDataConflict, collection, id)
					}
				}
			}
		}
	}
	return nil
}
