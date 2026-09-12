package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/jxsl13/perfscan/traceevidence"
)

// Reject duplicate keys and null at every nesting level before decoding the
// typed inventory. A byte bound alone does not make ambiguous policy safe.
func decodeInputPolicy(data []byte) (*traceevidence.InputPolicy, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return errors.New("input inventory nesting limit")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return errors.New("null is not an input inventory value")
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate or invalid input inventory key")
				}
				// encoding/json otherwise accepts case-fold aliases for tagged
				// struct fields. Admit exact canonical spellings only; the typed
				// decoder below independently checks each object's field context.
				switch name {
				case "inventoryComplete", "inputs", "additionalProtectedRoots", "maxInputBytes", "opened", "path", "sha256":
				default:
					return errors.New("unknown or noncanonical input inventory key")
				}
				seen[name] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected input inventory delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := value(0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing input inventory data")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var p traceevidence.InputPolicy
	if err := d.Decode(&p); err != nil {
		return nil, err
	}
	// Presence and semantic validation are also enforced by Capture. Here
	// require explicit fields, including [] for an audited empty inventory.
	if !p.InventoryComplete || p.Inputs == nil || p.MaxInputBytes < 1 || p.Opened == "" {
		return nil, errors.New("audited complete inventory, explicit inputs array, byte budget and input-open marker are required")
	}
	return &p, nil
}
