package handlers

import (
	"strings"
	"testing"
)

func TestParseCSVRows(t *testing.T) {
	in := strings.NewReader(`holder,degree,classification
Achieng Otieno,BSc Computer Science,First Class
,,
John Doe,MSc Data Science,Merit
Jane Smith,,Distinction
`)
	rows, header, err := parseCSVRows(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wantHeader := []string{"holder", "degree", "classification"}
	if len(header) != len(wantHeader) {
		t.Fatalf("header len: got %d want %d", len(header), len(wantHeader))
	}
	for i, c := range wantHeader {
		if header[i] != c {
			t.Fatalf("header[%d]: got %q want %q", i, header[i], c)
		}
	}
	if len(rows) != 3 {
		t.Fatalf("rows len: got %d want 3 (blank row dropped)", len(rows))
	}
	if rows[0]["holder"] != "Achieng Otieno" {
		t.Errorf("row[0] holder: %q", rows[0]["holder"])
	}
	if rows[2]["degree"] != "" {
		t.Errorf("row[2] degree should be empty, got %q", rows[2]["degree"])
	}
	if rows[2]["classification"] != "Distinction" {
		t.Errorf("row[2] classification: %q", rows[2]["classification"])
	}
}

func TestParseCSVRows_EmptyInput(t *testing.T) {
	_, _, err := parseCSVRows(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error on empty CSV, got nil")
	}
}
