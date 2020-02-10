package main

import (
	"flag"
	"github.com/inhies/go-bytesize"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	verbose := flag.Bool("v", false, "Verbose")
	veryVerbose := flag.Bool("vv", false, "Very Verbose")
	rootScanOnly := flag.Bool("root", false, "root scan only")
	targetClassName := flag.String("target", "", "Target class name")
	rlimitString := flag.String("rlimit", "4GB", "RLimit")
	memprofile := flag.String("memprofile", "", "write memory profile to `file`")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")

	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("Usage: heapdump path/to/heapdump.hprof")
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	heapFilePath := args[0]

	minLevel := LogLevel_INFO
	if *verbose {
		minLevel = LogLevel_DEBUG
	}
	if *veryVerbose {
		minLevel = LogLevel_TRACE
	}

	rlimitInt, err := bytesize.Parse(*rlimitString)
	if err != nil {
		log.Fatal(err)
	}
	var rLimit syscall.Rlimit
	err = syscall.Getrlimit(syscall.RLIMIT_AS, &rLimit)
	if err != nil {
		log.Fatal(err)
	}
	// TODO 調整可能なように
	rLimit.Cur = uint64(rlimitInt)
	rLimit.Max = uint64(rlimitInt)
	err = syscall.Setrlimit(syscall.RLIMIT_AS, &rLimit)
	if err != nil {
		log.Fatal(err)
	}

	logger := NewLogger(minLevel)

	// calculate the size of each instance objects.
	// 途中で sleep とか適宜入れる？
	analyzer, _ := NewHeapDumpAnalyzer(logger)
	{
		start := time.Now()
		err = analyzer.ReadFile(heapFilePath)
		if err != nil {
			log.Fatal(err)
		}
		elapsed := time.Since(start)
		logger.Info("Read heap dump file in %s.", elapsed)
	}

	rootScanner := NewRootScanner(logger)
	{
		start := time.Now()
		err := rootScanner.ScanAll(analyzer)
		if err != nil {
			log.Fatal(err)
		}
		elapsed := time.Since(start)
		logger.Info("Scanned retained root in %s.", elapsed)
	}

	if *rootScanOnly {
		os.Exit(0)
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}

	if targetClassName != nil && len(*targetClassName) > 0 {
		size, err := analyzer.CalculateRetainedSizeOfInstancesByName(*targetClassName, rootScanner)
		if err != nil {
			log.Fatal(err)
		}
		analyzer.logger.Info("ReadFile result: %v=%v", *targetClassName, size)
	} else {
		start := time.Now()
		err := analyzer.DumpInclusiveRanking(rootScanner)
		if err != nil {
			log.Fatalf("An error occurred: %v", err)
		}
		elapsed := time.Since(start)
		logger.Info("Calculated inclusive heap size in %s.", elapsed)
	}
}
