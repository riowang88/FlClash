package main

import "testing"

func TestHandlerRoutesRegister(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	NewServer(NewService(store, &Engine{})).Handler()
}
