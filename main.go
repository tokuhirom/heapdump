package main

import (
	"flag"
	"github.com/inhies/go-bytesize"
	"log"
	"os"
	"syscall"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	verbose := flag.Bool("v", false, "Verbose")
	veryVerbose := flag.Bool("vv", false, "Very Verbose")
	rootScanOnly := flag.Bool("root", false, "root scan only")
	targetClassName := flag.String("target", "", "Target class name")
	rlimitString := flag.String("rlimit", "4GB", "RLimit")

	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("Usage: heapdump path/to/heapdump.hprof")
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
	analyzer := NewHeapDumpAnalyzer(logger, true)
	err = analyzer.Scan(heapFilePath)
	if err != nil {
		log.Fatal(err)
	}

	rootScanner := NewRootScanner(logger)
	rootScanner.ScanAll(analyzer)

	if *rootScanOnly {
		os.Exit(0)
	}

	if targetClassName != nil && len(*targetClassName) > 0 {
		size := analyzer.CalculateSizeOfInstancesByName(*targetClassName, rootScanner)
		analyzer.logger.Info("Scan result: %v=%v", *targetClassName, size)
	} else {
		analyzer.DumpInclusiveRanking(rootScanner)
	}
}
