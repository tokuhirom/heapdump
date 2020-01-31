package main

import (
	"encoding/binary"
	"flag"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"github.com/hashicorp/logutils"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"syscall"
)

type HeapDumpAnalyzer struct {
	nameId2string                    map[uint64]string
	classObjectId2classNameId        map[uint64]uint64
	classObjectId2objectIds          map[uint64][]uint64
	classObjectId2classDump          map[uint64]*hprofdata.HProfClassDump
	arrayObjectId2primitiveArrayDump map[uint64]*hprofdata.HProfPrimitiveArrayDump
	arrayObjectId2objectArrayDump    map[uint64]*hprofdata.HProfObjectArrayDump
	countClassDump                   uint64 // Total Classes
	objectId2instanceDump            map[uint64]*hprofdata.HProfInstanceDump
	logger                           *Logger
	debug                            bool
	sizeCache                        map[uint64]uint64
	isRoot                           map[uint64]bool
	rootJniGlobals                   map[uint64]bool // 本当は slice にしたいがなんか動かないので。。
	rootJniLocal                     map[uint64]bool
	rootJavaFrame                    map[uint64]bool
	rootStickyClass                  map[uint64]bool
	rootThreadObj                    map[uint64]bool
	rootMonitorUsed                  map[uint64]bool
}

func NewHeapDumpAnalyzer(debug bool) *HeapDumpAnalyzer {
	m := new(HeapDumpAnalyzer)
	m.nameId2string = make(map[uint64]string)
	m.classObjectId2classNameId = make(map[uint64]uint64)
	m.classObjectId2objectIds = make(map[uint64][]uint64)
	m.classObjectId2classDump = make(map[uint64]*hprofdata.HProfClassDump)
	m.arrayObjectId2primitiveArrayDump = make(map[uint64]*hprofdata.HProfPrimitiveArrayDump)
	m.arrayObjectId2objectArrayDump = make(map[uint64]*hprofdata.HProfObjectArrayDump)
	m.objectId2instanceDump = make(map[uint64]*hprofdata.HProfInstanceDump)
	m.logger = NewLogger()
	m.debug = debug
	m.sizeCache = make(map[uint64]uint64)
	m.isRoot = make(map[uint64]bool)
	m.rootJniGlobals = make(map[uint64]bool)
	m.rootJniLocal = make(map[uint64]bool)
	m.rootJavaFrame = make(map[uint64]bool)
	m.rootStickyClass = make(map[uint64]bool)
	m.rootThreadObj = make(map[uint64]bool)
	m.rootMonitorUsed = make(map[uint64]bool)
	return m
}

func (a HeapDumpAnalyzer) Scan(heapFilePath string) error {
	a.logger.Info("Opening %v", heapFilePath)

	f, err := os.Open(heapFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	p := parser.NewParser(f)
	_, err = p.ParseHeader()
	if err != nil {
		return nil
	}

	var prev int64
	for {
		record, err := p.ParseRecord()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Got parsing issue: %v", err)
			continue
		}
		if pos, err := f.Seek(0, 1); err == nil && pos-prev > (1<<30) {
			log.Printf("currently %d GiB", pos/(1<<30))
			prev = pos
		}

		//var key uint64
		switch o := record.(type) {
		case *hprofdata.HProfRecordUTF8:
			//log.Printf("%v", o.GetNameId())
			//log.Printf("%v", o.XXX_Size())
			a.nameId2string[o.GetNameId()] = string(o.GetName())
			//key = o.GetNameId()
		case *hprofdata.HProfRecordLoadClass:
			/*
			 *                u4        class serial number (> 0)
			 *                id        class object ID
			 *                u4        stack trace serial number
			 *                id        class name ID
			 */
			//key = uint64(o.GetClassSerialNumber())
			a.classObjectId2classNameId[o.GetClassObjectId()] = o.GetClassNameId()
			//log.Printf("%v=%v", o.GetClassObjectId(),
			//	a.nameId2string[o.GetClassNameId()])
		case *hprofdata.HProfRecordFrame:
			// stack frame.
			//key = o.GetStackFrameId()
		case *hprofdata.HProfRecordTrace:
			// stack trace
			//key = uint64(o.GetStackTraceSerialNumber())
		case *hprofdata.HProfRecordHeapDumpBoundary:
			break
		case *hprofdata.HProfClassDump:
			//key = o.GetClassObjectId()
			//classNameId := classObjectId2classNameId[o.GetClassObjectId()]
			//className := nameId2string[classNameId]
			//log.Printf("className=%s", className)
			a.classObjectId2classDump[o.ClassObjectId] = o
			a.countClassDump += 1
		case *hprofdata.HProfInstanceDump: // HPROF_GC_INSTANCE_DUMP
			a.classObjectId2objectIds[o.ClassObjectId] = append(a.classObjectId2objectIds[o.ClassObjectId], o.ObjectId)
			a.objectId2instanceDump[o.ObjectId] = o
		case *hprofdata.HProfObjectArrayDump:
			arrayObjectId := o.GetArrayObjectId()
			a.arrayObjectId2objectArrayDump[arrayObjectId] = o
		case *hprofdata.HProfPrimitiveArrayDump:
			arrayObjectId := o.GetArrayObjectId()
			a.arrayObjectId2primitiveArrayDump[arrayObjectId] = o
		case *hprofdata.HProfRootJNIGlobal:
			//key = cs.countJNIGlobal
			//cs.countJNIGlobal++
			//a.rootObjectId[o.GetObjectId()] = true
			a.logger.Debug("Found JNI Global: %v", o.GetObjectId())
			a.rootJniGlobals[o.GetObjectId()] = true
		case *hprofdata.HProfRootJNILocal:
			//key = cs.countJNILocal
			//cs.countJNILocal++
			a.rootJniLocal[o.GetObjectId()] = true
		case *hprofdata.HProfRootJavaFrame:
			//key = cs.countJavaFrame
			//cs.countJavaFrame++
			a.rootJavaFrame[o.GetObjectId()] = true
		case *hprofdata.HProfRootStickyClass:
			//key = cs.countStickyClass
			//cs.countStickyClass++
			a.rootStickyClass[o.GetObjectId()] = true
		case *hprofdata.HProfRootThreadObj:
			//key = cs.countThreadObj
			//cs.countThreadObj++
			a.rootThreadObj[o.GetThreadObjectId()] = true
		case *hprofdata.HProfRootMonitorUsed:
			a.rootMonitorUsed[o.GetObjectId()] = true
		default:
			log.Printf("unknown record type!!: %#v", record)
		}
	}
	return nil
}

func (a HeapDumpAnalyzer) DumpInclusiveRanking(rootScanner *RootScanner) {
	log.Printf("[INFO] --- DumpInclusiveRanking")
	var classObjectIds []uint64
	for k, _ := range a.classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}

	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectIds[i] < classObjectIds[j]
	})

	var classObjectId2objectSize = map[uint64]uint64{}
	classObjectId2objectCount := make(map[uint64]int)
	for _, classObjectId := range classObjectIds {
		objectIds := a.classObjectId2objectIds[classObjectId]
		classNameId := a.classObjectId2classNameId[classObjectId]
		name := a.nameId2string[classNameId]

		if strings.HasPrefix(name, "jdk/internal/") {
			a.logger.Debug("Skip internal class: %v", name)
			continue
		}

		for _, objectId := range objectIds {
			a.logger.Info("Starting scan %v(classObjectId=%v, objectId=%v)\n",
				name, classObjectId, objectId)

			size := a.GetRetainedSize(objectId, rootScanner)
			classObjectId2objectSize[classObjectId] += size

			a.logger.Info("Finished scan %v(classObjectId=%v, objectId=%v) size=%v\n",
				name, classObjectId, objectId, size)
		}
		classObjectId2objectCount[classObjectId] = len(objectIds)
	}

	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectId2objectSize[classObjectIds[i]] < classObjectId2objectSize[classObjectIds[j]]
	})
	for _, classObjectId := range classObjectIds {
		classNameId := a.classObjectId2classNameId[classObjectId]
		name := a.nameId2string[classNameId]
		log.Printf("[INFO] %10d(count=%5d)= %s\n",
			classObjectId2objectSize[classObjectId],
			classObjectId2objectCount[classObjectId],
			name)
	}
}

func (a HeapDumpAnalyzer) DumpExclusiveRanking() {
	var classObjectIds []uint64
	for k, _ := range a.classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}
	sort.Slice(classObjectIds, func(i, j int) bool {
		return len(a.classObjectId2objectIds[classObjectIds[i]]) < len(a.classObjectId2objectIds[classObjectIds[j]])
	})
	for _, classObjectId := range classObjectIds {
		classNameId := a.classObjectId2classNameId[classObjectId]
		name := a.nameId2string[classNameId]
		log.Printf("[INFO] %d\t= %s\n",
			len(a.classObjectId2objectIds[classObjectId]),
			name)
	}
}

func (a HeapDumpAnalyzer) ShowTotalClasses() {
	log.Printf("Total Classes=%v", a.countClassDump)
}

func (a HeapDumpAnalyzer) GetRetainedSize(objectId uint64, rootScanner *RootScanner) uint64 {
	rootObjectId := rootScanner.GetNearestGcRoot(objectId)
	seen := NewSeen()
	return a.retainedSizeInstance(rootObjectId, objectId, seen, rootScanner)
}

func (a HeapDumpAnalyzer) retainedSizeInstance(rootObjectId uint64, objectId uint64, seen *Seen, rootScanner *RootScanner) uint64 {
	if seen == nil {
		panic("Missing seen")
	}
	if seen.HasKey(objectId) { // recursive counting.
		a.logger.Debug("Recursive counting occurred: %v", objectId)
		return 0
	}

	if size, ok := a.sizeCache[objectId]; ok {
		return size
	}

	seen.Add(objectId)
	//if seen.Size() > 3000 {
	//	panic("Too much seen.")
	//}

	a.logger.Indent()
	defer a.logger.Dedent()

	instanceDump := a.objectId2instanceDump[objectId]
	if instanceDump != nil {
		name := a.nameId2string[a.classObjectId2classNameId[instanceDump.ClassObjectId]]
		// XXX why??
		if name == "java/lang/String" {
			// String is a special class.
			return 0
		}
		// XXX why??
		if name == "java/lang/Module" {
			// String is a special class.
			return 0
		}

		a.logger.Debug("retainedSizeInstance(%v) objectId=%d seen=%v",
			name,
			objectId,
			seen.Size())
		return a.calcObjectSize(instanceDump, objectId, seen, rootObjectId, rootScanner)
	}

	a.logger.Debug("retainedSizeInstance() objectId=%d seen=%v",
		objectId,
		seen.Size())

	objectArrayDump := a.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return a.calcObjectArraySize(objectArrayDump, seen, rootObjectId, rootScanner)
	}

	primitiveArrayDump := a.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		return a.calcPrimitiveArraySize(primitiveArrayDump)
	}

	classDump := a.classObjectId2classDump[objectId]
	if classDump != nil {
		return a.calcClassSize(classDump, seen)
	}

	log.Fatalf(
		"[ERROR] Unknown instance: objectId=%v instanceDump=%v str=%v primArray=%v objArray=%v class=%v",
		objectId,
		instanceDump,
		a.nameId2string[objectId],
		a.arrayObjectId2primitiveArrayDump[objectId],
		a.arrayObjectId2objectArrayDump[objectId],
		a.classObjectId2classNameId[objectId])
	return 0 // should not reach here
}

func (a HeapDumpAnalyzer) calcObjectSize(
	instanceDump *hprofdata.HProfInstanceDump,
	objectId uint64,
	seen *Seen,
	rootObjectId uint64,
	rootScanner *RootScanner) uint64 {
	classDump := a.classObjectId2classDump[instanceDump.ClassObjectId]
	classNameId := a.classObjectId2classNameId[classDump.ClassObjectId]
	a.logger.Debug("calcObjectSize(name=%v) oid=%d",
		a.nameId2string[classNameId],
		objectId)

	// instance field を舐めてサイズを計算する。super があればそこも全てみる。

	// 全てのインスタンスは 16 byte かかるっぽい。
	// https://weekly-geekly.github.io/articles/447848/index.html
	// On a 64-bit jvm, the object header consists of 16 bytes. Arrays are additionally 4 bytes.

	size := uint64(16 + len(instanceDump.GetValues()))
	values := instanceDump.GetValues()
	// 読み込んだバイト数, サイズ
	for {
		name := a.nameId2string[a.classObjectId2classNameId[classDump.ClassObjectId]]
		a.logger.Trace("scan instance: name=%v",
			name)

		nIdx, nSize := a.scanInstance(classDump.InstanceFields, values, seen, name, rootObjectId, rootScanner)
		size += nSize
		a.logger.Trace("nSize=%v (%v)", nSize,
			a.nameId2string[a.classObjectId2classNameId[classDump.ClassObjectId]])
		values = values[nIdx:]
		if classDump.SuperClassObjectId == 0 {
			break
		} else {
			classDump = a.classObjectId2classDump[classDump.SuperClassObjectId]
			a.logger.Trace("checking super class %v",
				a.nameId2string[a.classObjectId2classNameId[classDump.ClassObjectId]])
		}
	}

	a.logger.Debug("/calcObjectSize(name=%v) oid=%d size=%v seen=%v",
		a.nameId2string[classNameId],
		objectId,
		size, seen)

	a.sizeCache[objectId] = size
	return size
}

func (a HeapDumpAnalyzer) scanInstance(
	fields []*hprofdata.HProfClassDump_InstanceField,
	values []byte,
	seen *Seen,
	className string,
	rootObjectId uint64,
	rootScanner *RootScanner) (int, uint64) {

	size := uint64(0)
	idx := 0

	for _, field := range fields {
		nameId := field.NameId
		if field.Type == hprofdata.HProfValueType_OBJECT {
			// TODO 32bit support(特にやる気なし)
			objectIdBytes := values[idx : idx+8]
			objectId := binary.BigEndian.Uint64(objectIdBytes)
			if objectId == 0 {
				// the field contains null
				a.logger.Trace("object field: className=%v name=%v oid=NULL",
					className,
					a.nameId2string[nameId])
			} else {
				a.logger.Trace("start:: object field: className=%v name=%v oid=%v",
					className,
					a.nameId2string[nameId],
					objectId)
				n := a.retainedSizeInstance(rootObjectId, objectId, seen, rootScanner)
				if rootScanner.GetNearestGcRoot(objectId) == rootObjectId {
					a.logger.Trace("finished:: object field: className=%v name=%v oid=%v, size=%v",
						className,
						a.nameId2string[nameId],
						objectId, n)
					size += n
				} else {
					a.logger.Trace("IGNORE!!:: object field: className=%v name=%v oid=%v size=%v",
						className,
						a.nameId2string[nameId],
						objectId, n)
				}
			}
			idx += 8
		} else {
			a.logger.Trace("Primitive Field: %v", a.nameId2string[nameId])
			idx += parser.ValueSize[field.Type]
		}
	}

	return idx, size
}

func (a HeapDumpAnalyzer) scanStaticFields(
	fields []*hprofdata.HProfClassDump_StaticField,
	seen *Seen,
	rootObjectId uint64,
	rootScanner *RootScanner) uint64 {
	size := uint64(0)
	for _, field := range fields {
		switch field.GetType() {
		case hprofdata.HProfValueType_OBJECT:
			// TODO 32bit support(特にやる気なし)
			objectId := field.GetValue()
			log.Printf("[TRACE]  object field: name=%v oid=%v",
				a.nameId2string[field.GetNameId()],
				objectId)
			if objectId == 0 {
				// the field contains null
			} else {
				size += a.retainedSizeInstance(rootObjectId, objectId, seen, rootScanner)
			}
			size += 8
		// Boolean. Takes 0 or 1. One byte.
		case hprofdata.HProfValueType_BOOLEAN:
			log.Printf("[TRACE]  Boolean Field: %v", a.nameId2string[field.GetNameId()])
			size += 1
		// Character. Two bytes.
		case hprofdata.HProfValueType_CHAR:
			log.Printf("[TRACE]  char Field: %v", a.nameId2string[field.GetNameId()])
			size += 2
		// Float. 4 bytes
		case hprofdata.HProfValueType_FLOAT:
			log.Printf("[TRACE]  float Field: %v", a.nameId2string[field.GetNameId()])
			size += 4
		// Double. 8 bytes.
		case hprofdata.HProfValueType_DOUBLE:
			log.Printf("[TRACE]  double Field: %v", a.nameId2string[field.GetNameId()])
			size += 8
		// Byte. One byte.
		case hprofdata.HProfValueType_BYTE:
			log.Printf("[TRACE]  byte Field: %v", a.nameId2string[field.GetNameId()])
			size += 1
		// Short. Two bytes.
		case hprofdata.HProfValueType_SHORT:
			log.Printf("[TRACE]  short Field: %v %v",
				a.nameId2string[field.GetNameId()],
				int16(field.GetValue()))
			size += 2
		// Integer. 4 bytes.
		case hprofdata.HProfValueType_INT:
			log.Printf("[TRACE]  int Field: %v, %v",
				a.nameId2string[field.GetNameId()],
				int32(field.GetValue()))
			size += 4
		// Long. 8 bytes.
		case hprofdata.HProfValueType_LONG:
			log.Printf("[TRACE]  long Field: %v",
				a.nameId2string[field.GetNameId()])
			size += 8
		default:
			log.Fatalf("Unknown value type: %x", field.GetType())
		}
	}
	return size
}

func (a HeapDumpAnalyzer) CalculateSizeOfInstancesByName(targetName string, rootScanner *RootScanner) map[uint64]uint64 {
	// Debugging
	retval := make(map[uint64]uint64)
	for classObjectId, objectIds := range a.classObjectId2objectIds {
		name := a.nameId2string[a.classObjectId2classNameId[classObjectId]]
		if name == targetName {
			for _, objectId := range objectIds {
				a.logger.Debug("**** Scanning %v", targetName)
				size := a.GetRetainedSize(objectId, rootScanner)
				retval[objectId] = size
				a.logger.Debug("**** Scanned %v\n\n", size)
			}
			break
		}
	}

	if a.debug {
		totalSize := uint64(0)
		var sizeList []uint64
		for _, size := range retval {
			totalSize += size
			sizeList = append(sizeList, size)
		}
		sort.Slice(sizeList, func(i, j int) bool {
			return sizeList[i] > sizeList[j]
		})
		a.logger.Debug("--- scanning total report totalSize=%v len=%v %v\n\n",
			totalSize, len(sizeList), sizeList)
	}

	return retval
}

func (a HeapDumpAnalyzer) calcObjectArraySize(dump *hprofdata.HProfObjectArrayDump, seen *Seen, rootObjectId uint64,
	rootScanner *RootScanner) uint64 {
	a.logger.Debug("--- calcObjectArraySize")

	objectIds := dump.GetElementObjectIds()
	// TODO 24 バイトのヘッダがついてるっぽい。length 用だけなら 8 バイトで良さそうだが、なぜか？
	// 8 = 64bit
	r := uint64(24 + 8*len(dump.GetElementObjectIds()))
	var sizeResult []uint64
	for _, objectId := range objectIds {
		if objectId != 0 {
			s := a.retainedSizeInstance(rootObjectId, objectId, seen, rootScanner)
			if a.debug {
				sizeResult = append(sizeResult, s)
			}
			r += s
		}
	}
	a.logger.Debug("object array: %v len=%v size=%v sizeResult=%v",
		dump.ArrayObjectId,
		len(dump.GetElementObjectIds()),
		r, sizeResult)
	return r
}

func (a HeapDumpAnalyzer) calcPrimitiveArraySize(dump *hprofdata.HProfPrimitiveArrayDump) uint64 {
	size := parser.ValueSize[dump.ElementType]
	// https://weekly-geekly.github.io/articles/447848/index.html
	// On a 64-bit jvm, the object header consists of 16 bytes. Arrays are additionally 4 bytes.
	// http://btoddb-java-sizing.blogspot.com/
	retval := uint64(16 + 4 + 4 + (len(dump.Values) * size))
	a.logger.Debug("primitive array: %s %v",
		dump.ElementType,
		retval)
	return retval
}

func (a HeapDumpAnalyzer) calcClassSize(dump *hprofdata.HProfClassDump, seen *Seen) uint64 {
	log.Printf("[DEBUG]      class: %v",
		dump.ClassObjectId)
	//return a.scanStaticFields(dump.GetStaticFields(), seen)
	return 0
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	verbose := flag.Bool("v", false, "Verbose")
	veryVerbose := flag.Bool("vv", false, "Very Verbose")
	rootScanOnly := flag.Bool("root", false, "root scan only")
	targetClassName := flag.String("target", "", "Target class name")

	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("Usage: heapdump path/to/heapdump.hprof")
	}

	heapFilePath := args[0]

	minLevel := "INFO"
	if *verbose {
		minLevel = "DEBUG"
	}
	if *veryVerbose {
		minLevel = "TRACE"
	}

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel(minLevel),
		Writer:   os.Stdout,
	}
	log.SetOutput(filter)

	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_AS, &rLimit)
	if err != nil {
		log.Fatal(err)
	}
	// TODO 調整可能なように
	rLimit.Cur = 4 * 1000_000_000
	rLimit.Max = 4 * 1000_000_000
	err = syscall.Setrlimit(syscall.RLIMIT_AS, &rLimit)
	if err != nil {
		log.Fatal(err)
	}

	// calculate the size of each instance objects.
	// 途中で sleep とか適宜入れる？
	analyzer := NewHeapDumpAnalyzer(true)
	err = analyzer.Scan(heapFilePath)
	if err != nil {
		log.Fatal(err)
	}

	rootScanner := NewRootScanner(analyzer.logger)
	rootScanner.ScanAll(analyzer)

	if *rootScanOnly {
		os.Exit(0)
	}

	if targetClassName != nil && len(*targetClassName) > 0 {
		analyzer.CalculateSizeOfInstancesByName(*targetClassName, rootScanner)
	} else {
		analyzer.DumpInclusiveRanking(rootScanner)
	}
}
