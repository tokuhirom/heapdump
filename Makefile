all: testdata/bytearray/heapdump.hprof testdata/empty/heapdump.hprof testdata/int/heapdump.hprof testdata/recursion/heapdump.hprof testdata/object/heapdump.hprof testdata/array/heapdump.hprof testdata/class/heapdump.hprof testdata/string/heapdump.hprof testdata/hashmap/heapdump.hprof testdata/boxed/heapdump.hprof testdata/stringbuilder/heapdump.hprof

testdata/empty/heapdump.hprof: testdata/empty/TestData.java
	cd testdata/empty && javac TestData.java && java TestData heapdump.hprof

testdata/int/heapdump.hprof: testdata/int/TestData.java
	cd testdata/int && javac TestData.java && java TestData heapdump.hprof

testdata/recursion/heapdump.hprof: testdata/recursion/TestData.java
	cd testdata/recursion && javac TestData.java && java TestData heapdump.hprof

testdata/object/heapdump.hprof: testdata/object/TestData.java
	cd testdata/object && javac TestData.java && java TestData heapdump.hprof

testdata/array/heapdump.hprof: testdata/array/TestData.java
	cd testdata/array && javac TestData.java && java TestData heapdump.hprof

testdata/class/heapdump.hprof: testdata/class/TestData.java
	cd testdata/class && javac TestData.java && java TestData heapdump.hprof

testdata/string/heapdump.hprof: testdata/string/TestData.java
	cd testdata/string && javac TestData.java && java TestData heapdump.hprof

testdata/hashmap/heapdump.hprof: testdata/hashmap/TestData.java
	cd testdata/hashmap && javac TestData.java && java TestData heapdump.hprof

testdata/boxed/heapdump.hprof: testdata/boxed/TestData.java
	cd testdata/boxed && javac TestData.java && java TestData heapdump.hprof

testdata/stringbuilder/heapdump.hprof: testdata/stringbuilder/TestData.java
	cd testdata/stringbuilder && javac TestData.java && java TestData heapdump.hprof

testdata/bytearray/heapdump.hprof: testdata/bytearray/TestData.java
	cd testdata/bytearray && javac TestData.java && java TestData heapdump.hprof

.PHONY: all
