package main

import (
	"testing"
)

type Tester struct {
	analyzer *HeapDumpAnalyzer
	t        *testing.T
}

func NewTester(path string, t *testing.T) *Tester {
	m := new(Tester)
	m.t = t
	m.analyzer, _ = NewHeapDumpAnalyzer(NewLogger(LogLevel_INFO))
	err := m.analyzer.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func (a *Tester) AssertSize(targetClass string, expectedRetainedSize uint64) {
	rootScanner := NewRootScanner(a.analyzer.logger)
	rootScanner.ScanAll(a.analyzer)
	sizeMap, err := a.analyzer.CalculateRetainedSizeOfInstancesByName(targetClass, rootScanner)
	if err != nil {
		a.t.Fatal(err)
	}

	var sizeList []uint64
	for _, size := range sizeMap {
		sizeList = append(sizeList, size)
	}

	if len(sizeList) != 1 {
		a.t.Fatalf("Only %v %v instance exists",
			len(sizeList),
			targetClass)
	}
	if sizeList[0] != expectedRetainedSize {
		a.t.Fatalf("%v instance should be %v bytes(visualvm says). But %v",
			targetClass,
			expectedRetainedSize,
			sizeList[0])
	}
}

func (a *Tester) GetTotalSize(targetClass string) (uint64, error) {
	rootScanner := NewRootScanner(a.analyzer.logger)
	rootScanner.ScanAll(a.analyzer)
	sizeMap, err := a.analyzer.CalculateRetainedSizeOfInstancesByName(targetClass, rootScanner)
	if err != nil {
		return 0, err
	}

	totalSize := uint64(0)
	for _, size := range sizeMap {
		totalSize += size
	}
	return totalSize, nil
}

func (a *Tester) AssertTotalSize(targetClass string, expectedRetainedSize uint64) {
	totalSize, err := a.GetTotalSize(targetClass)
	if err != nil {
		a.t.Fatal(err)
	}

	if totalSize != expectedRetainedSize {
		a.t.Fatalf("%v instance should be %v bytes(visualvm says). But %v",
			targetClass,
			expectedRetainedSize,
			totalSize)
	}
}

func (a *Tester) AssertTotalSizeLessThan(targetClass string, expectedRetainedSize uint64) {
	totalSize, err := a.GetTotalSize(targetClass)
	if err != nil {
		a.t.Fatal(err)
	}

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
	testInstanceSize(t, "testdata/empty/heapdump.hprof", "Object1", 16)
}

func TestInt(t *testing.T) {
	testInstanceSize(t, "testdata/int/heapdump.hprof", "Object1", 20)
}

func TestObject(t *testing.T) {
	testInstanceSize(t, "testdata/object/heapdump.hprof", "Object2", 42)
	testInstanceSize(t, "testdata/object/heapdump.hprof", "Object1", 66)
}

func TestRecursion(t *testing.T) {
	testInstanceSize(t, "testdata/recursion/heapdump.hprof", "Object2", 24)
	testInstanceSize(t, "testdata/recursion/heapdump.hprof", "Object1", 48)
}

func TestArray(t *testing.T) {
	t.Skip("broken test.")

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
	t.Skip("broken test.")

	tester := NewTester("testdata/boxed/heapdump.hprof", t)
	tester.AssertSize("Object1", 24)
}

func TestHashMap(t *testing.T) {
	t.Skip("broken test.")

	tester := NewTester("testdata/hashmap/heapdump.hprof", t)
	tester.AssertSize("Object1", 24)
}

func TestStringBuilder(t *testing.T) {
	tester := NewTester("testdata/stringbuilder/heapdump.hprof", t)
	tester.AssertSize("Object1", 93)
}

func TestByteArray(t *testing.T) {
	tester := NewTester("testdata/bytearray/heapdump.hprof", t)
	tester.AssertSize("Object1", 53)
}
