package codec_test

import (
	"testing"

	"github.com/chelinho139/wow-ai/go-bridge/internal/codec"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	payload := []byte("hello\x1fsession\x1fchat\x1f1")
	cells := codec.Encode(42, payload)
	msg, err := codec.DecodeCells(cells, codec.CellsPerRow, codec.MaxRows)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != 42 {
		t.Fatalf("id %d", msg.ID)
	}
	if string(msg.Payload) != string(payload) {
		t.Fatalf("payload %q", msg.Payload)
	}
}

func TestCellColor(t *testing.T) {
	r, g, b := codec.CellColor(7)
	if r != 1 || g != 1 || b != 1 {
		t.Fatalf("got %v %v %v", r, g, b)
	}
	r, g, b = codec.CellColor(0)
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("got %v %v %v", r, g, b)
	}
}

func TestRejectBadMagic(t *testing.T) {
	cells := make([]byte, 200)
	_, err := codec.DecodeCells(cells, 200, 1)
	if err != codec.ErrNoMagic {
		t.Fatalf("want ErrNoMagic, got %v", err)
	}
}

func TestFletcherMatchesEncode(t *testing.T) {
	cells := codec.Encode(1, []byte("x"))
	msg, err := codec.DecodeCells(cells, 200, 48)
	if err != nil {
		t.Fatal(err)
	}
	if string(msg.Payload) != "x" {
		t.Fatal(msg.Payload)
	}
}
