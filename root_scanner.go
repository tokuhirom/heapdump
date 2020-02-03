package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"log"
)

type RootScanner struct {
	parents map[uint64]map[uint64]bool // objectId -> count
	logger  *Logger
}

func NewRootScanner(logger *Logger) *RootScanner {
	m := new(RootScanner)
	m.parents = make(map[uint64]map[uint64]bool)
	m.logger = logger
	return m
}

func (r RootScanner) ScanRoot(a *HeapDumpAnalyzer, rootObjectIds []uint64) {
	r.logger.Debug("--- ScanRoot ---: %v", len(rootObjectIds))
	seen := NewSeen()
	for _, rootObjectId := range rootObjectIds {
		r.logger.Debug("rootObjectId=%v", rootObjectId)
		r.scan(rootObjectId, a, seen)
	}
	r.logger.Debug("--- /ScanRoot --- %v %v")
}

func (r RootScanner) scan(objectId uint64, a *HeapDumpAnalyzer, seen *Seen) {
	if objectId == 0 {
		return // NULL
	}
	if seen.HasKey(objectId) {
		return
	}

	a.logger.Indent()
	defer a.logger.Dedent()

	seen.Add(objectId)

	instanceDump := a.hprof.objectId2instanceDump[objectId]
	if instanceDump != nil {
		r.logger.Debug("instance dump = %v", objectId)

		classDump := a.hprof.classObjectId2classDump[instanceDump.ClassObjectId]
		values := instanceDump.GetValues()
		idx := 0

		for {
			for _, instanceField := range classDump.InstanceFields {
				if instanceField.Type == hprofdata.HProfValueType_OBJECT {
					// TODO 32bit support
					r.logger.Trace("instance field = %v.%v", instanceDump.ObjectId,
						a.hprof.nameId2string[instanceField.NameId])
					objectIdBytes := values[idx : idx+8]
					childObjectId := binary.BigEndian.Uint64(objectIdBytes)
					r.RegisterParent(objectId, childObjectId)
					r.scan(childObjectId, a, seen)
					idx += 8
				} else {
					idx += parser.ValueSize[instanceField.Type]
				}
			}
			classDump = a.hprof.classObjectId2classDump[classDump.SuperClassObjectId]
			if classDump == nil {
				break
			}
		}
		return
	}

	classDump := a.hprof.classObjectId2classDump[objectId]
	if classDump != nil {
		// scan super
		r.logger.Debug("class dump = %v", objectId)

		idx := 0
		for _, field := range classDump.StaticFields {
			if field.Type == hprofdata.HProfValueType_OBJECT {
				childObjectId := field.GetValue()
				r.RegisterParent(objectId, childObjectId)
				r.scan(childObjectId, a, seen)
				idx += 8
			} else {
				idx += parser.ValueSize[field.Type]
			}
		}

		// scan super class
		super := a.hprof.classObjectId2classDump[classDump.SuperClassObjectId]
		if super != nil {
			r.RegisterParent(objectId, super.ClassObjectId)
			r.scan(super.ClassObjectId, a, seen)
		}
		return
	}

	// object array
	objectArrayDump := a.hprof.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		r.logger.Debug("object array = %v", objectId)
		for _, childObjectId := range objectArrayDump.ElementObjectIds {
			r.RegisterParent(objectId, childObjectId)
			r.scan(childObjectId, a, seen)
		}
		return
	}

	// primitive array
	primitiveArrayDump := a.hprof.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		r.logger.Debug("primitive array = %v", objectId)
		return
	}

	log.Fatalf("SHOULD NOT REACH HERE: %v pa=%v oa=%v id=%v cd=%v",
		objectId,
		a.hprof.arrayObjectId2primitiveArrayDump[objectId],
		a.hprof.arrayObjectId2objectArrayDump[objectId],
		a.hprof.objectId2instanceDump[objectId],
		a.hprof.classObjectId2classDump[objectId])
}

func (r RootScanner) RegisterParent(parentObjectId uint64, childObjectId uint64) {
	if _, ok := r.parents[childObjectId]; !ok {
		r.parents[childObjectId] = make(map[uint64]bool)
	}
	r.logger.Trace("RegisterParent: parentObjectId=%v childObjectId=%v distance=%v currentDistance=%v",
		parentObjectId, childObjectId)
	r.parents[childObjectId][parentObjectId] = true
}

/**
Returns true if the `objectId` is referenced from only the parent object.
*/
func (r RootScanner) IsRetained(parentObjectId uint64, childObjectId uint64) bool {
	parentObjectIds, ok := r.parents[childObjectId]
	if !ok {
		return false
	}
	if len(parentObjectIds) > 1 {
		return false
	}
	for key := range parentObjectIds {
		return key == parentObjectId
	}
	panic("SHOULD NOT REACH HERE")
}

func (r RootScanner) ScanAll(analyzer *HeapDumpAnalyzer) {
	r.logger.Info("Scanning retained root")
	r.ScanRoot(analyzer, keys(analyzer.hprof.rootJniGlobals))
	r.ScanRoot(analyzer, keys(analyzer.hprof.rootJniLocal))
	r.ScanRoot(analyzer, keys(analyzer.hprof.rootJavaFrame))
	r.ScanRoot(analyzer, keys(analyzer.hprof.rootStickyClass))
	r.ScanRoot(analyzer, keys(analyzer.hprof.rootThreadObj))
	r.ScanRoot(analyzer, keys(analyzer.hprof.rootMonitorUsed))
}
