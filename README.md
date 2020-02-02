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

* いろんな種類のオブジェクトで、ちゃんとした値が算出されるようにする
* HTML Report
* String の挙動確認
* hashmap の容量の計算がうまくいってない
* logger にインデントの概念を導入する

https://gist.github.com/arturmkrtchyan/43d6135e8a15798cc46c

http://btoddb-java-sizing.blogspot.com/

https://github.com/openjdk/jdk/blob/6be7841937944364d365b33a795e7aa89dac2c58/src/hotspot/share/services/heapDumper.cpp

https://github.com/openjdk/jdk/blob/6be7841937944364d365b33a795e7aa89dac2c58/src/hotspot/share/services/heapDumper.cpp

https://github.com/oracle/visualvm
https://github.com/bpupadhyaya/openjdk-8/tree/master/jdk/src/share/classes/com/sun/tools/hat
https://openjdk.java.net/jeps/241