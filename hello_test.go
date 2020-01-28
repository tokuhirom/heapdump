package main

import (
	"testing"
)

func TestEmpty(t *testing.T) {
	analyzer := NewHeapDumpAnalyzer()
	err := analyzer.Scan("testdata/empty/empty.hprof")
	if err != nil {
		t.Fatal(err)
	}
	sizeMap := analyzer.CalculateSizeOfInstancesByName("Empty")

	var sizeList []uint64
	for _, size := range sizeMap {
		sizeList = append(sizeList, size)
	}

	if len(sizeList) != 1 {
		t.Fatal("Only 1 Empty instance exists")
	}
	if sizeList[0] != 16 {
		t.Fatalf("Empty instance should be 16 bytes(visualvm says). But %v",
			sizeList[0])
	}
}

func TestInt(t *testing.T) {
	analyzer := NewHeapDumpAnalyzer()
	err := analyzer.Scan("testdata/int/int.hprof")
	if err != nil {
		t.Fatal(err)
	}
	sizeMap := analyzer.CalculateSizeOfInstancesByName("IntHolder")

	var sizeList []uint64
	for _, size := range sizeMap {
		sizeList = append(sizeList, size)
	}

	if len(sizeList) != 1 {
		t.Fatal("Only 1 Empty instance exists")
	}
	if sizeList[0] != 20 {
		t.Fatalf("Empty instance should be 20 bytes(visualvm says). But %v",
			sizeList[0])
	}
}
