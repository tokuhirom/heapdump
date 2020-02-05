package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"golang.org/x/text/message"
	"log"
	"sort"
)

type HeapDumpAnalyzer struct {
	logger *Logger
	hprof *HProf
	softSizeCalculator *SoftSizeCalculator
	sizeCache map[uint64]uint64
}

func NewHeapDumpAnalyzer(logger *Logger, debug bool) *HeapDumpAnalyzer {
	m := new(HeapDumpAnalyzer)
	m.logger = logger
	m.sizeCache = make(map[uint64]uint64)
	m.hprof = NewHProf(logger)
	m.softSizeCalculator = NewSoftSizeCalculator(logger)
	return m
}

func (a HeapDumpAnalyzer) ReadFile(heapFilePath string) error {
	return a.hprof.ReadFile(heapFilePath)
}

func (a HeapDumpAnalyzer) DumpInclusiveRanking(rootScanner *RootScanner) {
	a.logger.Debug("DumpInclusiveRanking")
	var classObjectIds []uint64
	for k := range a.hprof.classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}

	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectIds[i] < classObjectIds[j]
	})

	var classObjectId2objectSize = map[uint64]uint64{}
	classObjectId2objectCount := make(map[uint64]int)
	for _, classObjectId := range classObjectIds {
		objectIds := a.hprof.classObjectId2objectIds[classObjectId]
		classNameId := a.hprof.classObjectId2classNameId[classObjectId]
		name := a.hprof.nameId2string[classNameId]

		for _, objectId := range objectIds {
			a.logger.Debug("Starting scan %v(classObjectId=%v, objectId=%v)\n",
				name, classObjectId, objectId)

			size := a.GetRetainedSize(objectId, rootScanner)
			classObjectId2objectSize[classObjectId] += size

			a.logger.Debug("Finished scan %v(classObjectId=%v, objectId=%v) size=%v\n",
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
		classNameId := a.hprof.classObjectId2classNameId[classObjectId]
		name := a.hprof.nameId2string[classNameId]
		a.logger.Info(p.Sprintf("shallowSize=%10d retainedSize=%10d(count=%5d)= %s",
			a.softSizeCalculator.CalcSoftSizeByClassObjectId(a.hprof, classObjectId),
			classObjectId2objectSize[classObjectId],
			classObjectId2objectCount[classObjectId],
			name))
	}
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

	instanceDump := a.hprof.objectId2instanceDump[objectId]
	if instanceDump != nil {
		name := a.hprof.nameId2string[a.hprof.classObjectId2classNameId[instanceDump.ClassObjectId]]

		a.logger.Debug("retainedSizeInstance(%v) objectId=%d seen=%v",
			name,
			objectId,
			seen.Size())
		return a.calcObjectSize(instanceDump, objectId, seen, rootScanner)
	}

	a.logger.Debug("retainedSizeInstance() objectId=%d seen=%v",
		objectId,
		seen.Size())

	objectArrayDump := a.hprof.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return a.calcObjectArraySize(objectArrayDump, seen, rootScanner)
	}

	primitiveArrayDump := a.hprof.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		return a.calcPrimitiveArraySize(primitiveArrayDump)
	}

	classDump := a.hprof.classObjectId2classDump[objectId]
	if classDump != nil {
		return a.calcClassSize(classDump, seen, rootScanner)
	}

	log.Fatalf(
		"[ERROR] Unknown instance: objectId=%v instanceDump=%v str=%v primArray=%v objArray=%v class=%v",
		objectId,
		instanceDump,
		a.hprof.nameId2string[objectId],
		a.hprof.arrayObjectId2primitiveArrayDump[objectId],
		a.hprof.arrayObjectId2objectArrayDump[objectId],
		a.hprof.classObjectId2classNameId[objectId])
	return 0 // should not reach here
}

func (a HeapDumpAnalyzer) calcObjectSize(
	instanceDump *hprofdata.HProfInstanceDump,
	objectId uint64,
	seen *Seen,
	rootScanner *RootScanner) uint64 {
	classDump := a.hprof.classObjectId2classDump[instanceDump.ClassObjectId]
	classNameId := a.hprof.classObjectId2classNameId[classDump.ClassObjectId]
	a.logger.Debug("calcObjectSize(name=%v) oid=%d",
		a.hprof.nameId2string[classNameId],
		objectId)

	// instance field を舐めてサイズを計算する。super があればそこも全てみる。

	// 全てのインスタンスは 16 byte かかるっぽい。
	// https://weekly-geekly.github.io/articles/447848/index.html
	// On a 64-bit jvm, the object header consists of 16 bytes. Arrays are additionally 4 bytes.

	size := uint64(16 + len(instanceDump.GetValues()))
	values := instanceDump.GetValues()
	// 読み込んだバイト数, サイズ
	for {
		name := a.hprof.nameId2string[a.hprof.classObjectId2classNameId[classDump.ClassObjectId]]
		a.logger.Trace("scan instance: name=%v",
			name)

		nIdx, nSize := a.scanInstance(classDump.InstanceFields, values, seen, name, objectId, rootScanner)
		size += nSize
		a.logger.Trace("nSize=%v (%v)", nSize,
			a.hprof.nameId2string[a.hprof.classObjectId2classNameId[classDump.ClassObjectId]])
		values = values[nIdx:]
		if classDump.SuperClassObjectId == 0 {
			break
		} else {
			classDump = a.hprof.classObjectId2classDump[classDump.SuperClassObjectId]
			a.logger.Trace("checking super class %v",
				a.hprof.nameId2string[a.hprof.classObjectId2classNameId[classDump.ClassObjectId]])
		}
	}

	a.logger.Debug("/calcObjectSize(name=%v) oid=%d size=%v seen=%v",
		a.hprof.nameId2string[classNameId],
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
					a.hprof.nameId2string[nameId])
			} else {
				a.logger.Trace("start:: object field: className=%v name=%v oid=%v",
					className,
					a.hprof.nameId2string[nameId],
					childObjectId)
				if rootScanner.IsRetained(parentObjectId, childObjectId) {
					n := a.retainedSizeInstance(childObjectId, seen, rootScanner)
					a.logger.Trace("finished:: object field: className=%v name=%v oid=%v, size=%v",
						className,
						a.hprof.nameId2string[nameId],
						childObjectId, n)
					size += n
				} else {
					a.logger.Trace("IGNORE!!:: object field: className=%v name=%v oid=%v",
						className,
						a.hprof.nameId2string[nameId],
						childObjectId)
				}
			}
			idx += 8
		} else {
			a.logger.Trace("Primitive Field: %v", a.hprof.nameId2string[nameId])
			idx += parser.ValueSize[field.Type]
		}
	}

	return idx, size
}

func (a HeapDumpAnalyzer) CalculateRetainedSizeOfInstancesByName(targetName string, rootScanner *RootScanner) map[uint64]uint64 {
	objectID2size := make(map[uint64]uint64)

	for classObjectId, objectIds := range a.hprof.classObjectId2objectIds {
		name := a.hprof.nameId2string[a.hprof.classObjectId2classNameId[classObjectId]]
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
	a.logger.Debug("calcClassSize: %v",
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
