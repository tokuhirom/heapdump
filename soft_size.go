package main

import (
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
)

type SoftSizeCalculator struct {
	logger *Logger
}

func NewSoftSizeCalculator(logger *Logger) *SoftSizeCalculator {
	m := new(SoftSizeCalculator)
	m.logger = logger
	return m
}

func (s SoftSizeCalculator) CalcSoftSizeByClassObjectId(hprof *HProf, classObjectId uint64) (int, error) {
	size := 0
	for _, objectId := range hprof.classObjectId2objectIds[classObjectId] {
		n, err := s.CalcSoftSizeByObjectId(hprof, objectId)
		if err != nil {
			return 0, err
		}
		size += n
	}
	return size, nil
}

func (s SoftSizeCalculator) CalcSoftSizeByObjectId(hprof *HProf, objectId uint64) (int, error) {
	instanceDump := hprof.objectId2instanceDump[objectId]
	if instanceDump != nil {
		return 16 + len(instanceDump.Values), nil
	}

	classDump, err := hprof.GetClassObjectByClassObjectId(objectId)
	if err != nil {
		return 0, err
	}
	if classDump != nil {
		idx := 0
		for _, field := range classDump.StaticFields {
			if field.Type == hprofdata.HProfValueType_OBJECT {
				idx += 8
			} else {
				idx += parser.ValueSize[field.Type]
			}
		}
		return idx, nil
	}

	// object array
	objectArrayDump := hprof.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return len(objectArrayDump.ElementObjectIds) * 8, nil
	}

	// primitive array
	primitiveArrayDump := hprof.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		return len(primitiveArrayDump.Values) * parser.ValueSize[primitiveArrayDump.ElementType], nil
	}

	s.logger.Fatalf("SHOULD NOT REACH HERE: %v pa=%v oa=%v id=%v cd=%v",
		objectId,
		hprof.arrayObjectId2primitiveArrayDump[objectId],
		hprof.arrayObjectId2objectArrayDump[objectId],
		hprof.objectId2instanceDump[objectId])
	return -1, nil // should not reach here
}
