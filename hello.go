package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"golang.org/x/text/message"
	"io"
	"log"
	"os"
	"sort"
)

type HeapDumpAnalyzer struct {
	logger *Logger

	nameId2string map[uint64]string

	classObjectId2classNameId        map[uint64]uint64
	classObjectId2objectIds          map[uint64][]uint64
	classObjectId2classDump          map[uint64]*hprofdata.HProfClassDump
	arrayObjectId2primitiveArrayDump map[uint64]*hprofdata.HProfPrimitiveArrayDump
	arrayObjectId2objectArrayDump    map[uint64]*hprofdata.HProfObjectArrayDump
	objectId2instanceDump            map[uint64]*hprofdata.HProfInstanceDump

	sizeCache map[uint64]uint64

	rootJniGlobals  map[uint64]bool // 本当は slice にしたいがなんか動かないので。。
	rootJniLocal    map[uint64]bool
	rootJavaFrame   map[uint64]bool
	rootStickyClass map[uint64]bool
	rootThreadObj   map[uint64]bool
	rootMonitorUsed map[uint64]bool
}

func NewHeapDumpAnalyzer(logger *Logger, debug bool) *HeapDumpAnalyzer {
	m := new(HeapDumpAnalyzer)

	m.logger = logger

	m.nameId2string = make(map[uint64]string)

	m.classObjectId2classNameId = make(map[uint64]uint64)
	m.classObjectId2objectIds = make(map[uint64][]uint64)
	m.classObjectId2classDump = make(map[uint64]*hprofdata.HProfClassDump)
	m.arrayObjectId2primitiveArrayDump = make(map[uint64]*hprofdata.HProfPrimitiveArrayDump)
	m.arrayObjectId2objectArrayDump = make(map[uint64]*hprofdata.HProfObjectArrayDump)
	m.objectId2instanceDump = make(map[uint64]*hprofdata.HProfInstanceDump)

	m.sizeCache = make(map[uint64]uint64)

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
			a.logger.Warn("Got parsing issue: %v", err)
			continue
		}
		if pos, err := f.Seek(0, 1); err == nil && pos-prev > (1<<30) {
			a.logger.Info("currently %d GiB", pos/(1<<30))
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
	for k := range a.classObjectId2objectIds {
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

	// sort by retained size
	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectId2objectSize[classObjectIds[i]] < classObjectId2objectSize[classObjectIds[j]]
	})

	// print result
	p := message.NewPrinter(message.MatchLanguage("en"))
	for _, classObjectId := range classObjectIds {
		classNameId := a.classObjectId2classNameId[classObjectId]
		name := a.nameId2string[classNameId]
		log.Printf(p.Sprintf("[INFO] shallowSize=%10d retainedSize=%10d(count=%5d)= %s\n",
			a.calcSoftSizeByClassObjectId(classObjectId),
			classObjectId2objectSize[classObjectId],
			classObjectId2objectCount[classObjectId],
			name))
	}
}

func (a HeapDumpAnalyzer) calcSoftSizeByClassObjectId(classObjectId uint64) int {
	size := 0
	for _, objectId := range a.classObjectId2objectIds[classObjectId] {
		size += a.calcSoftSize(objectId)
	}
	return size
}

func (a HeapDumpAnalyzer) calcSoftSize(objectId uint64) int {
	instanceDump := a.objectId2instanceDump[objectId]
	if instanceDump != nil {
		return 16 + len(instanceDump.Values)
	}

	classDump := a.classObjectId2classDump[objectId]
	if classDump != nil {
		idx := 0
		for _, field := range classDump.StaticFields {
			if field.Type == hprofdata.HProfValueType_OBJECT {
				idx += 8
			} else {
				idx += parser.ValueSize[field.Type]
			}
		}
		return idx
	}

	// object array
	objectArrayDump := a.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return len(objectArrayDump.ElementObjectIds) * 8
	}

	// primitive array
	primitiveArrayDump := a.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		return len(primitiveArrayDump.Values) * parser.ValueSize[primitiveArrayDump.ElementType]
	}

	log.Fatalf("SHOULD NOT REACH HERE: %v pa=%v oa=%v id=%v cd=%v",
		objectId,
		a.arrayObjectId2primitiveArrayDump[objectId],
		a.arrayObjectId2objectArrayDump[objectId],
		a.objectId2instanceDump[objectId],
		a.classObjectId2classDump[objectId])
	return -1 // should not reach here
}

func (a HeapDumpAnalyzer) GetRetainedSize(objectId uint64, rootScanner *RootScanner) uint64 {
	seen := NewSeen()
	return a.retainedSizeInstance(objectId, seen, rootScanner)
}

func (a HeapDumpAnalyzer) getSizeCache(objectId uint64) (uint64, bool) {
	size, ok := a.sizeCache[objectId]
	return size, ok
}

func (a HeapDumpAnalyzer) setSizeCache(objectId uint64, size uint64) {
	a.sizeCache[objectId] = size
}

func (a HeapDumpAnalyzer) retainedSizeInstance(objectId uint64, seen *Seen, rootScanner *RootScanner) uint64 {
	if seen == nil {
		panic("Missing seen")
	}
	if seen.HasKey(objectId) { // recursive counting.
		a.logger.Debug("Recursive counting occurred: %v", objectId)
		return 0
	}

	if size, ok := a.getSizeCache(objectId); ok {
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

		a.logger.Debug("retainedSizeInstance(%v) objectId=%d seen=%v",
			name,
			objectId,
			seen.Size())
		return a.calcObjectSize(instanceDump, objectId, seen, rootScanner)
	}

	a.logger.Debug("retainedSizeInstance() objectId=%d seen=%v",
		objectId,
		seen.Size())

	objectArrayDump := a.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return a.calcObjectArraySize(objectArrayDump, seen, rootScanner)
	}

	primitiveArrayDump := a.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		return a.calcPrimitiveArraySize(primitiveArrayDump)
	}

	classDump := a.classObjectId2classDump[objectId]
	if classDump != nil {
		return a.calcClassSize(classDump, seen, rootScanner)
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

		nIdx, nSize := a.scanInstance(classDump.InstanceFields, values, seen, name, objectId, rootScanner)
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

	a.setSizeCache(objectId, size)
	return size
}

func (a HeapDumpAnalyzer) scanInstance(
	fields []*hprofdata.HProfClassDump_InstanceField,
	values []byte,
	seen *Seen,
	className string,
	parentObjectId uint64,
	rootScanner *RootScanner) (int, uint64) {

	size := uint64(0)
	idx := 0

	for _, field := range fields {
		nameId := field.NameId
		if field.Type == hprofdata.HProfValueType_OBJECT {
			// TODO 32bit support(特にやる気なし)
			objectIdBytes := values[idx : idx+8]
			childObjectId := binary.BigEndian.Uint64(objectIdBytes)
			if childObjectId == 0 {
				// the field contains null
				a.logger.Trace("object field: className=%v name=%v oid=NULL",
					className,
					a.nameId2string[nameId])
			} else {
				a.logger.Trace("start:: object field: className=%v name=%v oid=%v",
					className,
					a.nameId2string[nameId],
					childObjectId)
				if rootScanner.IsRetained(parentObjectId, childObjectId) {
					n := a.retainedSizeInstance(childObjectId, seen, rootScanner)
					a.logger.Trace("finished:: object field: className=%v name=%v oid=%v, size=%v",
						className,
						a.nameId2string[nameId],
						childObjectId, n)
					size += n
				} else {
					a.logger.Trace("IGNORE!!:: object field: className=%v name=%v oid=%v",
						className,
						a.nameId2string[nameId],
						childObjectId)
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

func (a HeapDumpAnalyzer) CalculateRetainedSizeOfInstancesByName(targetName string, rootScanner *RootScanner) map[uint64]uint64 {
	objectID2size := make(map[uint64]uint64)

	for classObjectId, objectIds := range a.classObjectId2objectIds {
		name := a.nameId2string[a.classObjectId2classNameId[classObjectId]]
		if name == targetName {
			for _, objectId := range objectIds {
				a.logger.Debug("**** Scanning %v objectId=%v", targetName, objectId)
				size := a.GetRetainedSize(objectId, rootScanner)
				objectID2size[objectId] = size
				a.logger.Debug("**** Scanned %v\n\n", size)
			}
			break
		}
	}

	return objectID2size
}

func (a HeapDumpAnalyzer) calcObjectArraySize(dump *hprofdata.HProfObjectArrayDump, seen *Seen,
	rootScanner *RootScanner) uint64 {
	a.logger.Debug("--- calcObjectArraySize")

	objectIds := dump.GetElementObjectIds()
	// TODO 24 バイトのヘッダがついてるっぽい。length 用だけなら 8 バイトで良さそうだが、なぜか？
	// 8 = 64bit
	r := uint64(24 + 8*len(dump.GetElementObjectIds()))
	var sizeResult []uint64
	for _, objectId := range objectIds {
		if objectId != 0 {
			if rootScanner.IsRetained(dump.ArrayObjectId, objectId) {
				s := a.retainedSizeInstance(objectId, seen, rootScanner)
				if a.logger.IsDebugEnabled() {
					sizeResult = append(sizeResult, s)
				}
				r += s
			}
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

func (a HeapDumpAnalyzer) calcClassSize(dump *hprofdata.HProfClassDump, seen *Seen, rootScanner *RootScanner) uint64 {
	log.Printf("[DEBUG]      class: %v",
		dump.ClassObjectId)

	totalSize := uint64(0)
	for _, field := range dump.StaticFields {
		if field.Type == hprofdata.HProfValueType_OBJECT {
			childObjectId := field.Value
			totalSize += 8
			if childObjectId != 0 {
				if rootScanner.IsRetained(dump.ClassObjectId, childObjectId) {
					size := a.retainedSizeInstance(childObjectId, seen, rootScanner)
					totalSize += size
				}
			}
		} else {
			totalSize += uint64(parser.ValueSize[field.Type])
		}
	}
	return totalSize
}
