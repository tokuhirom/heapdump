package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"github.com/hashicorp/logutils"
	"io"
	"log"
	"os"
	"sort"
)

type Seen struct {
	seen map[uint64]bool
}

func NewSeen() *Seen {
	m := new(Seen)
	m.seen = make(map[uint64]bool)
	return m
}

func (s Seen) Add(id uint64) {
	s.seen[id] = true
}

func (s Seen) Remove(id uint64) {
	delete(s.seen, id)
}

func (s Seen) HasKey(id uint64) bool {
	_, ok := s.seen[id]
	return ok
}

func (s Seen) Size() int {
	return len(s.seen)
}

type HeapDumpAnalyzer struct {
	nameId2string                    map[uint64]string
	classObjectId2classNameId        map[uint64]uint64
	classObjectId2objectIds          map[uint64][]uint64
	classObjectId2classDump          map[uint64]*hprofdata.HProfClassDump
	arrayObjectId2primitiveArrayDump map[uint64]*hprofdata.HProfPrimitiveArrayDump
	arrayObjectId2objectArrayDump    map[uint64]*hprofdata.HProfObjectArrayDump
	countClassDump                   uint64 // Total Classes
	objectId2instanceDump            map[uint64]*hprofdata.HProfInstanceDump
	sizeCache                        map[uint64]uint64
}

func NewHeapDumpAnalyzer() *HeapDumpAnalyzer {
	m := new(HeapDumpAnalyzer)
	m.nameId2string = make(map[uint64]string)
	m.classObjectId2classNameId = make(map[uint64]uint64)
	m.classObjectId2objectIds = make(map[uint64][]uint64)
	m.classObjectId2classDump = make(map[uint64]*hprofdata.HProfClassDump)
	m.arrayObjectId2primitiveArrayDump = make(map[uint64]*hprofdata.HProfPrimitiveArrayDump)
	m.arrayObjectId2objectArrayDump = make(map[uint64]*hprofdata.HProfObjectArrayDump)
	m.objectId2instanceDump = make(map[uint64]*hprofdata.HProfInstanceDump)
	m.sizeCache = make(map[uint64]uint64)
	return m
}

func (a HeapDumpAnalyzer) Scan(heapFilePath string) error {
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
		case *hprofdata.HProfRootJNILocal:
			//key = cs.countJNILocal
			//cs.countJNILocal++
		case *hprofdata.HProfRootJavaFrame:
			//key = cs.countJavaFrame
			//cs.countJavaFrame++
		case *hprofdata.HProfRootStickyClass:
			//key = cs.countStickyClass
			//cs.countStickyClass++
		case *hprofdata.HProfRootThreadObj:
			//key = cs.countThreadObj
			//cs.countThreadObj++
		case *hprofdata.HProfRootMonitorUsed:
		default:
			log.Printf("unknown record type!!: %#v", record)
		}
	}
	return nil
}

func (a HeapDumpAnalyzer) DumpInclusiveRanking() {
	log.Printf("[INFO] --- DumpInclusiveRanking")
	var classObjectIds []uint64
	for k, _ := range a.classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}

	var classObjectId2objectSize = map[uint64]uint64{}
	classObjectId2objectCount := make(map[uint64]int)
	for classObjectId, objectIds := range a.classObjectId2objectIds {
		for _, objectId := range objectIds {
			classObjectId2objectSize[classObjectId] += a.retainedSizeInstance(objectId, NewSeen())
		}
		classObjectId2objectCount[classObjectId] = len(objectIds)
	}

	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectId2objectSize[classObjectIds[i]] < classObjectId2objectSize[classObjectIds[j]]
	})
	for _, classObjectId := range classObjectIds {
		classNameId := a.classObjectId2classNameId[classObjectId]
		name := a.nameId2string[classNameId]
		log.Printf("[INFO] %20d(count=%5d)= %s\n",
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

func (a HeapDumpAnalyzer) retainedSizeInstance(objectId uint64, seen *Seen) uint64 {
	if cacheEntry, ok := a.sizeCache[objectId]; ok {
		return cacheEntry
	}

	if seen == nil {
		panic("Missing seen")
	}
	if seen.HasKey(objectId) { // recursive counting.
		log.Printf("[DEBUG] Recursive counting occurred: %v", objectId)
		return 0
	}

	log.Printf("[DEBUG] retainedSizeInstance() objectId=%d seen=%v",
		objectId,
		seen)

	seen.Add(objectId)
	defer seen.Remove(objectId)

	instanceDump := a.objectId2instanceDump[objectId]
	if instanceDump != nil {
		if a.nameId2string[a.classObjectId2classNameId[instanceDump.ClassObjectId]] == "java/lang/String" {
			// String is a special class.
			return 0
		}
		return a.calcObjectSize(instanceDump, objectId, seen)
	}

	objectArrayDump := a.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return a.calcObjectArraySize(objectArrayDump, seen)
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
	seen *Seen) uint64 {
	classDump := a.classObjectId2classDump[instanceDump.ClassObjectId]
	classNameId := a.classObjectId2classNameId[classDump.ClassObjectId]
	log.Printf("[DEBUG] calcObjectSize(name=%v) oid=%d seen=%v",
		a.nameId2string[classNameId],
		objectId,
		seen)

	// instance field を舐めてサイズを計算する。super があればそこも全てみる。

	// 全てのインスタンスは 16 byte かかるっぽい。

	size := uint64(16 + len(instanceDump.GetValues()))
	values := instanceDump.GetValues()
	// 読み込んだバイト数, サイズ
	for {
		nIdx, nSize := a.scanInstance(classDump.InstanceFields, values, seen)
		size += nSize
		log.Printf("[TRACE] nSize=%v (%v)", nSize,
			a.nameId2string[a.classObjectId2classNameId[classDump.ClassObjectId]])
		values = values[nIdx:]
		if classDump.SuperClassObjectId == 0 {
			break
		} else {
			classDump = a.classObjectId2classDump[classDump.SuperClassObjectId]
			log.Printf("[DEBUG]  checking super class %v",
				a.nameId2string[a.classObjectId2classNameId[classDump.ClassObjectId]])
		}
	}

	a.sizeCache[objectId] = size
	return size
}

type Fieldish interface {
	GetType() hprofdata.HProfValueType
	GetNameId() uint64
}

func (a HeapDumpAnalyzer) scanInstance(
	fields []*hprofdata.HProfClassDump_InstanceField,
	values []byte,
	seen *Seen) (int, uint64) {

	size := uint64(0)
	idx := 0

	for _, field := range fields {
		nameId := field.NameId
		switch field.Type {
		case hprofdata.HProfValueType_OBJECT:
			// TODO 32bit support(特にやる気なし)
			objectIdBytes := values[idx : idx+8]
			objectId := binary.BigEndian.Uint64(objectIdBytes)
			if objectId == 0 {
				// the field contains null
				log.Printf("[TRACE]  object field: name=%v oid=%v",
					a.nameId2string[nameId],
					objectId)
			} else {
				log.Printf("[TRACE]  start:: object field: name=%v oid=%v",
					a.nameId2string[nameId],
					objectId)
				n := a.retainedSizeInstance(objectId, seen)
				log.Printf("[TRACE]  finished:: object field: name=%v oid=%v, size=%v",
					a.nameId2string[nameId],
					objectId, n)
				size += n
			}
			idx += 8
		// Boolean. Takes 0 or 1. One byte.
		case hprofdata.HProfValueType_BOOLEAN:
			log.Printf("[TRACE]  boolean Field: %v", a.nameId2string[nameId])
			idx += 1
		// Character. Two bytes.
		case hprofdata.HProfValueType_CHAR:
			log.Printf("[TRACE]  char Field: %v", a.nameId2string[nameId])
			idx += 2
		// Float. 4 bytes
		case hprofdata.HProfValueType_FLOAT:
			log.Printf("[TRACE]  float Field: %v", a.nameId2string[nameId])
			idx += 4
		// Double. 8 bytes.
		case hprofdata.HProfValueType_DOUBLE:
			log.Printf("[TRACE]  double Field: %v", a.nameId2string[nameId])
			idx += 8
		// Byte. One byte.
		case hprofdata.HProfValueType_BYTE:
			log.Printf("[TRACE]  byte Field: %v", a.nameId2string[nameId])
			idx += 1
		// Short. Two bytes.
		case hprofdata.HProfValueType_SHORT:
			log.Printf("[TRACE]  short Field: %v %v",
				a.nameId2string[nameId],
				int16(binary.BigEndian.Uint16(values[idx:idx+2])))
			idx += 2
		// Integer. 4 bytes.
		case hprofdata.HProfValueType_INT:
			log.Printf("[TRACE]  int Field: %v, %v",
				a.nameId2string[nameId],
				int32(binary.BigEndian.Uint32(values[idx:idx+4])))
			idx += 4
		// Long. 8 bytes.
		case hprofdata.HProfValueType_LONG:
			log.Printf("[TRACE]  long Field: %v",
				a.nameId2string[nameId])
			idx += 8
		default:
			log.Fatalf("Unknown value type: %x", field.Type)
		}
	}

	return idx, size
}

func (a HeapDumpAnalyzer) scanStaticFields(
	fields []*hprofdata.HProfClassDump_StaticField,
	seen *Seen) uint64 {
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
				size += a.retainedSizeInstance(objectId, seen)
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

func (a HeapDumpAnalyzer) CalculateSizeOfInstancesByName(targetName string) map[uint64]uint64 {
	// Debugging
	retval := make(map[uint64]uint64)
	for classObjectId, objectIds := range a.classObjectId2objectIds {
		name := a.nameId2string[a.classObjectId2classNameId[classObjectId]]
		if name == targetName {
			for _, objectId := range objectIds {
				log.Printf("[DEBUG] **** Scanning %v", targetName)
				seen := NewSeen()
				size := a.retainedSizeInstance(objectId, seen)
				retval[objectId] = size
				log.Printf("[DEBUG] **** Scanned %v\n\n", size)
			}
			break
		}
	}
	return retval
}

func (a HeapDumpAnalyzer) CalculateSizeOfInstances() {
	log.Printf("[INFO] --- CalculateSizeOfInstances")
	var classObjectIds []uint64
	for k, _ := range a.classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}
	sort.Slice(classObjectIds, func(i, j int) bool {
		return a.classObjectId2classNameId[classObjectIds[i]] < a.classObjectId2classNameId[classObjectIds[j]]
	})

	for _, classObjectId := range classObjectIds {
		objectIds := a.classObjectId2objectIds[classObjectId]

		for _, objectId := range objectIds {
			seen := NewSeen()
			i := a.retainedSizeInstance(objectId, seen)
			log.Printf("[INFO] Scaned %d", i)
		}
	}
}

func (a HeapDumpAnalyzer) calcObjectArraySize(dump *hprofdata.HProfObjectArrayDump, seen *Seen) uint64 {
	log.Printf("[INFO] --- calcObjectArraySize")

	objectIds := dump.GetElementObjectIds()
	// TODO 24 バイトのヘッダがついてるっぽい。length 用だけなら 8 バイトで良さそうだが、なぜか？
	// 8 = 64bit
	r := uint64(24 + 8 * len(dump.GetElementObjectIds()))
	for _, objectId := range objectIds {
		if objectId != 0 {
			r += a.retainedSizeInstance(objectId, seen)
		}
	}
	log.Printf("[DEBUG]      object array: %v len=%v size=%v",
		dump.ArrayObjectId,
		len(dump.GetElementObjectIds()),
		r)
	return r
}

func (a HeapDumpAnalyzer) calcPrimitiveArraySize(dump *hprofdata.HProfPrimitiveArrayDump) uint64 {
	size := parser.ValueSize[dump.ElementType]
	retval := uint64(4 + (len(dump.Values) * size))
	log.Printf("[DEBUG]      primitive array: %s %v",
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

func parseIt() error {
	heapFilePath := "testdata/hashmap/heapdump.hprof"

	// calculate the size of each instance objects.
	// 途中で sleep とか適宜入れる？
	analyzer := NewHeapDumpAnalyzer()
	err := analyzer.Scan(heapFilePath)
	if err != nil {
		return err
	}

	//analyzer.CalculateSizeOfInstancesByName("java/security/BasicPermissionCollection")
	//analyzer.CalculateSizeOfInstancesByName("jdk/internal/module/IllegalAccessLogger")
	analyzer.CalculateSizeOfInstancesByName("Object1")
	//analyzer.CalculateSizeOfInstances()

	//analyzer.DumpInclusiveRanking()
	//analyzer.DumpExclusiveRanking()
	//analyzer.ShowTotalClasses()

	return nil
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel("TRACE"),
		Writer:   os.Stderr,
	}
	log.SetOutput(filter)

	err := parseIt()
	if err != nil {
		log.Fatal(err)
	}
}
