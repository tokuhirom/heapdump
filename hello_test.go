package main

import (
	"testing"
)

type Tester struct {
	analyzer *HeapDumpAnalyzer
	t        *testing.T
}

func NewTester(path string, t *testing.T) *Tester {
	m:= new(Tester)
	m.t = t
	m.analyzer = NewHeapDumpAnalyzer(false)
	err := m.analyzer.Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (a *Tester) AssertSize(targetClass string, expectedRetainedSize uint64) {
	sizeMap := a.analyzer.CalculateSizeOfInstancesByName(targetClass)

	var sizeList []uint64
	for _, size := range sizeMap {
		sizeList = append(sizeList, size)
	}

	if len(sizeList) != 1 {
		a.t.Fatalf("Only 1 %v instance exists",
			targetClass)
	}
	if sizeList[0] != expectedRetainedSize {
		a.t.Fatalf("%v instance should be %v bytes(visualvm says). But %v",
			targetClass,
			expectedRetainedSize,
			sizeList[0])
	}
}

func (a *Tester) GetTotalSize(targetClass string) uint64 {
	sizeMap := a.analyzer.CalculateSizeOfInstancesByName(targetClass)

	totalSize := uint64(0)
	for _, size := range sizeMap {
		totalSize += size
	}
	return totalSize
}

func (a *Tester) AssertTotalSize(targetClass string, expectedRetainedSize uint64) {
	totalSize := a.GetTotalSize(targetClass)

	if totalSize != expectedRetainedSize {
		a.t.Fatalf("%v instance should be %v bytes(visualvm says). But %v",
			targetClass,
			expectedRetainedSize,
			totalSize)
	}
}

func (a *Tester) AssertTotalSizeLessThan(targetClass string, expectedRetainedSize uint64) {
	totalSize := a.GetTotalSize(targetClass)

	if totalSize >= expectedRetainedSize {
		a.t.Fatalf("%v instance should be less than %v bytes. But %v",
			targetClass,
			expectedRetainedSize,
			totalSize)
	}
}

func testInstanceSize(
	t *testing.T,
	path string,
	targetClass string,
	expectedRetainedSize uint64) {
	tester := NewTester(path, t)
	tester.AssertSize(targetClass, expectedRetainedSize)
}

func TestEmpty(t *testing.T) {
	testInstanceSize(t, "testdata/empty/empty.hprof", "Empty", 16)
}

func TestInt(t *testing.T) {
	testInstanceSize(t, "testdata/int/int.hprof", "IntHolder", 20)
}

func TestObject(t *testing.T) {
	testInstanceSize(t, "testdata/object/heapdump.hprof", "Object2", 42)
	testInstanceSize(t, "testdata/object/heapdump.hprof", "Object1", 66)
}

func TestRecursion(t *testing.T) {
	// Recursion2 は visualvm だと 24 bytes として計算される。なぜか。
	testInstanceSize(t, "testdata/recursion/heapdump.hprof", "Recursion2", 48)
	testInstanceSize(t, "testdata/recursion/heapdump.hprof", "Recursion1", 48)
}

func TestArray(t *testing.T) {
	tester := NewTester("testdata/array/heapdump.hprof", t)
	tester.AssertTotalSize("Object2", 480)
	tester.AssertTotalSize("Object3", 0)
	tester.AssertSize("Object1", 692)
}

// 特定のクラスがデカくなりすぎてるのを確認する。
func TestMisc(t *testing.T) {
	tester := NewTester("testdata/array/heapdump.hprof", t)
	tester.AssertTotalSizeLessThan("java/util/Vector", 1000)
}

func TestClass(t *testing.T) {
	tester := NewTester("testdata/class/heapdump.hprof", t)
	tester.AssertSize("Object1", 24)
}

func TestString(t *testing.T) {
	tester := NewTester("testdata/string/heapdump.hprof", t)
	tester.AssertSize("Object1", 24)
}

func TestBoxed(t *testing.T) {
	tester := NewTester("testdata/boxed/heapdump.hprof", t)
	tester.AssertSize("Object1", 24)
}

func TestHashMap(t *testing.T) {
	tester := NewTester("testdata/hashmap/heapdump.hprof", t)
	tester.AssertSize("Object1", 24)
}

