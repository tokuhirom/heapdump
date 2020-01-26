package main

import (
	"fmt"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"io"
	"log"
	"os"
	"sort"
)

type counters struct {
	countJNIGlobal   uint64
	countJNILocal    uint64
	countJavaFrame   uint64
	countStickyClass uint64
	countThreadObj   uint64
	countLoadClass   uint64
}

type HeapDumpAnalyzer struct {
	nameId2string             map[uint64]string
	classObjectId2classNameId map[uint64]uint64
	classObjectId2objectIds   map[uint64][]uint64
	classObjectId2classDump   map[uint64]*hprofdata.HProfClassDump
	arrayObjectId2bytes       map[uint64]int
	countClassDump            uint64 // Total Classes
	objectId2instanceDump     map[uint64]*hprofdata.HProfInstanceDump
}

func NewHeapDumpAnalyzer() *HeapDumpAnalyzer {
	m := new(HeapDumpAnalyzer)
	m.nameId2string = make(map[uint64]string)
	m.classObjectId2classNameId = make(map[uint64]uint64)
	m.classObjectId2objectIds = make(map[uint64][]uint64)
	m.classObjectId2classDump = make(map[uint64]*hprofdata.HProfClassDump)
	m.arrayObjectId2bytes = make(map[uint64]int)
	m.objectId2instanceDump = make(map[uint64]*hprofdata.HProfInstanceDump)
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

	cs := &counters{}
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
			log.Printf("%v=%v", o.GetClassObjectId(),
				a.nameId2string[o.GetClassNameId()])
			cs.countLoadClass += 1
		case *hprofdata.HProfRecordFrame:
			// stack frame.
			//key = o.GetStackFrameId()
		case *hprofdata.HProfRecordTrace:
			// stack trace
			//key = uint64(o.GetStackTraceSerialNumber())
		case *hprofdata.HProfRecordHeapDumpBoundary:
			log.Printf("DO IT")
			break
		case *hprofdata.HProfClassDump:
			//key = o.GetClassObjectId()
			//classNameId := classObjectId2classNameId[o.GetClassObjectId()]
			//className := nameId2string[classNameId]
			//log.Printf("className=%s", className)
			a.classObjectId2classDump[o.ClassObjectId] = o
			a.countClassDump += 1
		case *hprofdata.HProfInstanceDump: // HPROF_GC_INSTANCE_DUMP
			//key = o.GetObjectId()
			classNameId := a.classObjectId2classNameId[o.GetClassObjectId()]
			className := a.nameId2string[classNameId]
			log.Printf("INSTANCE! className=%s", className)
			a.classObjectId2objectIds[o.ClassObjectId] = append(a.classObjectId2objectIds[o.ClassObjectId], o.ObjectId)
			a.objectId2instanceDump[o.ObjectId] = o
		case *hprofdata.HProfObjectArrayDump:
			//key = o.GetArrayObjectId()
		case *hprofdata.HProfPrimitiveArrayDump:
			//key = o.GetArrayObjectId()
			arrayObjectId := o.GetArrayObjectId()
			a.arrayObjectId2bytes[arrayObjectId] = len(o.GetValues())
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

func (a HeapDumpAnalyzer) Dump() {
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
		fmt.Printf("%d\t= %s\n",
			len(a.classObjectId2objectIds[classObjectId]),
			name)
	}

	log.Printf("Total Classes=%v", a.countClassDump)
}

func (a HeapDumpAnalyzer) sizeInstance(objectId uint64) {
	instanceDump := a.objectId2instanceDump[objectId]
	classDump := a.classObjectId2classDump[instanceDump.ClassObjectId]
	log.Printf("%v", classDump)

	// instance field を舐めてサイズを計算する。super があればそこも全てみる。
	size := 0
	for _, field := range classDump.InstanceFields {
		switch field.Type {
		case hprofdata.HProfValueType_OBJECT:
			log.Printf("TODO: object scan is not available yet.")
		// Boolean. Takes 0 or 1. One byte.
		case hprofdata.HProfValueType_BOOLEAN:
			size += 1
			// Character. Two bytes.
		case hprofdata.HProfValueType_CHAR:
			size += 2
			// Float. 4 bytes
		case hprofdata.HProfValueType_FLOAT:
			size += 4
			// Double. 8 bytes.
		case hprofdata.HProfValueType_DOUBLE:
			size += 8
			// Byte. One byte.
		case hprofdata.HProfValueType_BYTE:
			size += 1
			// Short. Two bytes.
		case hprofdata.HProfValueType_SHORT:
			size += 2
			// Integer. 4 bytes.
		case hprofdata.HProfValueType_INT:
			size += 4
			// Long. 8 bytes.
		case hprofdata.HProfValueType_LONG:
			size += 8
		default:
			log.Fatalf("Unknown value type: %x", field.Type)
		}
	}
}

func (a HeapDumpAnalyzer) CalculateSizeOfInstances() {
	for _, objectIds := range a.classObjectId2objectIds {
		for _, objectId := range objectIds {
			a.sizeInstance(objectId)
		}
	}
}

func parseIt() error {
	heapFilePath := "/tmp/heapdump.hprof"

	// calculate the size of each instance objects.
	// 途中で sleep とか適宜入れる？
	analyzer := NewHeapDumpAnalyzer()
	err := analyzer.Scan(heapFilePath)
	if err != nil {
		return err
	}

	log.Printf("---- CalculateSizeOfInstances phase ----")
	analyzer.CalculateSizeOfInstances()

	analyzer.Dump()

	return nil
}

func main() {
	err := parseIt()
	if err != nil {
		log.Fatal(err)
	}
}
