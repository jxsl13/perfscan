package main

import "testing"

func TestDecodeInputPolicyRejectsCaseFoldAliases(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`{"inventoryComplete":true,"InventoryComplete":false,"inputs":[],"maxInputBytes":1,"opened":"OPEN"}`,
		`{"inventoryComplete":true,"inputs":[],"INPUTS":[],"maxInputBytes":1,"opened":"OPEN"}`,
		`{"inventoryComplete":true,"inputs":[{"path":"a","PATH":"b"}],"maxInputBytes":1,"opened":"OPEN"}`,
		`{"InventoryComplete":true,"inputs":[],"maxInputBytes":1,"opened":"OPEN"}`,
		`{"inventoryComplete":true,"inputs":[{"Path":"a"}],"maxInputBytes":1,"opened":"OPEN"}`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if _, err := decodeInputPolicy([]byte(source)); err == nil {
				t.Fatal("accepted encoding/json case-fold alias")
			}
		})
	}
}

func TestDecodeInputPolicy(t *testing.T) {
	t.Parallel()
	valid := `{"inventoryComplete":true,"inputs":[],"maxInputBytes":1073741824,"opened":"INPUTS_OPENED"}`
	if p, err := decodeInputPolicy([]byte(valid)); err != nil || !p.InventoryComplete || p.Inputs == nil {
		t.Fatalf("policy=%+v err=%v", p, err)
	}
	for _, source := range []string{`null`, `[]`, `{}`, `{"inventoryComplete":true,"inputs":null,"maxInputBytes":1,"opened":"OPEN"}`, `{"inventoryComplete":true,"inputs":[],"opened":"OPEN"}`, `{"inventoryComplete":false,"inputs":[],"maxInputBytes":1,"opened":"OPEN"}`, `{"inventoryComplete":true,"inventoryComplete":true,"inputs":[],"maxInputBytes":1,"opened":"OPEN"}`, `{"inventoryComplete":true,"inputs":[{"path":"a","path":"b"}],"maxInputBytes":1,"opened":"OPEN"}`, `{"inventoryComplete":true,"inputs":[{"path":"a","extra":1}],"maxInputBytes":1,"opened":"OPEN"}`, `{"inventoryComplete":true,"inputs":[],"maxInputBytes":1,"opened":"OPEN","extra":1}`, valid + ` {}`, `{"inventoryComplete":true,"inputs":[{"path":"a","sha256":null}],"maxInputBytes":1,"opened":"OPEN"}`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if _, err := decodeInputPolicy([]byte(source)); err == nil {
				t.Fatal("accepted ambiguous/omitted input policy")
			}
		})
	}
}
