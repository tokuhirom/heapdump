package main

import (
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"io"
	"os"
)

type HProf struct {
	logger *Logger

	nameId2string map[uint64]string

	classObjectId2classNameId        map[uint64]uint64
	classObjectId2objectIds          map[uint64][]uint64
	classObjectId2classDump          map[uint64]*hprofdata.HProfClassDump
	arrayObjectId2primitiveArrayDump map[uint64]*hprofdata.HProfPrimitiveArrayDump
	arrayObjectId2objectArrayDump    map[uint64]*hprofdata.HProfObjectArrayDump
	objectId2instanceDump            map[uint64]*hprofdata.HProfInstanceDump

	rootJniGlobals  map[uint64]bool // 本当は slice にしたいがなんか動かないので。。
	rootJniLocal    map[uint64]bool
	rootJavaFrame   map[uint64]bool
	rootStickyClass map[uint64]bool
	rootThreadObj   map[uint64]bool
	rootMonitorUsed map[uint64]bool
}

func NewHProf(logger *Logger) *HProf {
	m := new(HProf)

	m.logger = logger

	m.nameId2string = make(map[uint64]string)

	m.classObjectId2classNameId = make(map[uint64]uint64)
	m.classObjectId2objectIds = make(map[uint64][]uint64)
	m.classObjectId2classDump = make(map[uint64]*hprofdata.HProfClassDump)
	m.arrayObjectId2primitiveArrayDump = make(map[uint64]*hprofdata.HProfPrimitiveArrayDump)
	m.arrayObjectId2objectArrayDump = make(map[uint64]*hprofdata.HProfObjectArrayDump)
	m.objectId2instanceDump = make(map[uint64]*hprofdata.HProfInstanceDump)

	m.rootJniGlobals = make(map[uint64]bool)
	m.rootJniLocal = make(map[uint64]bool)
	m.rootJavaFrame = make(map[uint64]bool)
	m.rootStickyClass = make(map[uint64]bool)
	m.rootThreadObj = make(map[uint64]bool)
	m.rootMonitorUsed = make(map[uint64]bool)

	return m
}

func (h HProf) ReadFile(heapFilePath string) error {
	h.logger.Info("Opening %v", heapFilePath)

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
			h.logger.Warn("Got parsing issue: %v", err)
			continue
		}
		if pos, err := f.Seek(0, 1); err == nil && pos-prev > (1<<30) {
			h.logger.Info("currently %d GiB", pos/(1<<30))
			prev = pos
		}

		h.addRecord(record)
	}
	return nil
}

func (h HProf) addRecord(record interface{}) {
	switch o := record.(type) {
	case *hprofdata.HProfRecordUTF8:
		h.nameId2string[o.GetNameId()] = string(o.GetName())
	case *hprofdata.HProfRecordLoadClass:
		h.classObjectId2classNameId[o.GetClassObjectId()] = o.GetClassNameId()
	case *hprofdata.HProfRecordFrame:
		break
	case *hprofdata.HProfRecordTrace:
		break
	case *hprofdata.HProfRecordHeapDumpBoundary:
		break
	case *hprofdata.HProfClassDump:
		h.classObjectId2classDump[o.ClassObjectId] = o
	case *hprofdata.HProfInstanceDump: // HPROF_GC_INSTANCE_DUMP
		h.classObjectId2objectIds[o.ClassObjectId] = append(h.classObjectId2objectIds[o.ClassObjectId], o.ObjectId)
		h.objectId2instanceDump[o.ObjectId] = o
	case *hprofdata.HProfObjectArrayDump:
		arrayObjectId := o.GetArrayObjectId()
		h.arrayObjectId2objectArrayDump[arrayObjectId] = o
	case *hprofdata.HProfPrimitiveArrayDump:
		arrayObjectId := o.GetArrayObjectId()
		h.arrayObjectId2primitiveArrayDump[arrayObjectId] = o
	case *hprofdata.HProfRootJNIGlobal:
		h.rootJniGlobals[o.GetObjectId()] = true
	case *hprofdata.HProfRootJNILocal:
		h.rootJniLocal[o.GetObjectId()] = true
	case *hprofdata.HProfRootJavaFrame:
		h.rootJavaFrame[o.GetObjectId()] = true
	case *hprofdata.HProfRootStickyClass:
		h.rootStickyClass[o.GetObjectId()] = true
	case *hprofdata.HProfRootThreadObj:
		h.rootThreadObj[o.GetThreadObjectId()] = true
	case *hprofdata.HProfRootMonitorUsed:
		h.rootMonitorUsed[o.GetObjectId()] = true
	default:
		h.logger.Warn("unknown record type!!: %#v", record)
	}
}
