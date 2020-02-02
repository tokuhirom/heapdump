# heapdump

Heapdump analyzer based on [google/hprof-parser](https://github.com/google/hprof-parser).

## Goal

 1. Generate retained size based class histogram from hprof(heapdump file)
  * with less memory
  * and safe(using rlimit).
 2. Generate small 1 file index file from heap dump file.
 3. Share the analyzing results with team members.

## Note

 * class object ID -> class name ID
 * name ID -> string
 * class object ID -> list(object ID)
 * object ID -> list(field values)

というようなデータ構造を作成する。

## how do i run test?

    make # make test data
    go test

TODO:

* HTML Report

https://gist.github.com/arturmkrtchyan/43d6135e8a15798cc46c

http://btoddb-java-sizing.blogspot.com/

https://github.com/openjdk/jdk/blob/6be7841937944364d365b33a795e7aa89dac2c58/src/hotspot/share/services/heapDumper.cpp

https://github.com/openjdk/jdk/blob/6be7841937944364d365b33a795e7aa89dac2c58/src/hotspot/share/services/heapDumper.cpp

https://github.com/oracle/visualvm
https://github.com/bpupadhyaya/openjdk-8/tree/master/jdk/src/share/classes/com/sun/tools/hat
https://openjdk.java.net/jeps/241

    In MAT we followed the Lengauer & Tarjan algorithm
    https://www.eclipse.org/forums/index.php/t/531857/

https://medium.com/@chrishantha/basic-concepts-of-java-heap-dump-analysis-with-mat-e3615fd79eb

## What's Retained heap

 * https://help.eclipse.org/2019-12/index.jsp?topic=%2Forg.eclipse.mat.ui.help%2Fconcepts%2Fshallowretainedheap.html
 * https://www.ibm.com/support/knowledgecenter/ja/SS3KLZ/com.ibm.java.diagnostics.memory.analyzer.doc/shallowretainedheap.html
 * https://www.ibm.com/support/knowledgecenter/SS3KLZ/com.ibm.java.diagnostics.memory.analyzer.doc/shallowretainedheap.html
