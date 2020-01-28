all: testdata/empty/empty.hprof testdata/int/int.hprof

testdata/empty/empty.hprof: testdata/empty/EmptyTestData.java
	cd testdata/empty && javac EmptyTestData.java && java EmptyTestData empty.hprof

testdata/int/int.hprof: testdata/int/TestData.java
	cd testdata/int && javac TestData.java && java TestData int.hprof

.PHONY: all
