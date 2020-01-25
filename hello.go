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
	countClassDump   uint64 // Total Classes
}

func parseIt() error {
	heapFilePath := "/tmp/heapdump.hprof"

	f, err := os.Open(heapFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	p := parser.NewParser(f)
	_, err = p.ParseHeader()
	if err != nil {
		return err
	}

	nameId2string := make(map[uint64]string)
	classObjectId2classNameId := make(map[uint64]uint64)
	classObjectId2objectIds := make(map[uint64][]uint64)

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
			nameId2string[o.GetNameId()] = string(o.GetName())
			//key = o.GetNameId()
		case *hprofdata.HProfRecordLoadClass:
			/*
			 *                u4        class serial number (> 0)
			 *                id        class object ID
			 *                u4        stack trace serial number
			 *                id        class name ID
			 */
			//key = uint64(o.GetClassSerialNumber())
			classObjectId2classNameId[o.GetClassObjectId()] = o.GetClassNameId()
			log.Printf("%v=%v", o.GetClassObjectId(),
				nameId2string[o.GetClassNameId()])
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
			cs.countClassDump += 1
		case *hprofdata.HProfInstanceDump:
			//key = o.GetObjectId()
			classNameId := classObjectId2classNameId[o.GetClassObjectId()]
			className := nameId2string[classNameId]
			log.Printf("INSTANCE! className=%s", className)
			o.GetValues()
			classObjectId2objectIds[o.ClassObjectId] = append(classObjectId2objectIds[o.ClassObjectId], o.ObjectId)
		case *hprofdata.HProfObjectArrayDump:
			//key = o.GetArrayObjectId()
		case *hprofdata.HProfPrimitiveArrayDump:
			//key = o.GetArrayObjectId()
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
		default:
			log.Printf("unknown record type!: %#v", record)
		}
	}

	var classObjectIds []uint64
	for k, _ := range classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}
	sort.Slice(classObjectIds, func(i, j int) bool {
		return len(classObjectId2objectIds[classObjectIds[i]]) < len(classObjectId2objectIds[classObjectIds[j]])
	})
	for _, classObjectId := range classObjectIds {
		classNameId := classObjectId2classNameId[classObjectId]
		name := nameId2string[classNameId]
		fmt.Printf("%d\t= %s\n",
			len(classObjectId2objectIds[classObjectId]),
			name)
	}

	log.Printf("Total Classes=%v", cs.countClassDump)

	return nil
}

func main() {
	err := parseIt()
	if err != nil {
		log.Fatal(err)
	}
}
