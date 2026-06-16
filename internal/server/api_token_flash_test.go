package server

import "testing"

func TestAPITokenFlash_storeAndConsumeOnce(t *testing.T) {
	app := &App{}
	nonce, err := app.storeAPITokenFlash(42, "bs_test_token")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	token, ok := app.consumeAPITokenFlash(42, nonce)
	if !ok || token != "bs_test_token" {
		t.Fatalf("consume first: got %q ok=%v", token, ok)
	}
	_, ok = app.consumeAPITokenFlash(42, nonce)
	if ok {
		t.Fatal("expected flash to be single-use")
	}
}

func TestAPITokenFlash_wrongUser(t *testing.T) {
	app := &App{}
	nonce, err := app.storeAPITokenFlash(1, "bs_x")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, ok := app.consumeAPITokenFlash(2, nonce); ok {
		t.Fatal("expected wrong user to fail")
	}
}
