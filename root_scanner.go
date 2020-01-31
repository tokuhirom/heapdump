package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"log"
)

type RootScanner struct {
	nearestGcRoot  map[uint64]uint64 // objectId -> gc root's object ID
	gcRootDistance map[uint64]int    // objectId -> distance
	logger         *Logger
}

func NewRootScanner(logger *Logger) *RootScanner {
	m := new(RootScanner)
	m.nearestGcRoot = make(map[uint64]uint64)
	m.gcRootDistance = make(map[uint64]int)
	m.logger = logger
	return m
}

func (r RootScanner) ScanRoot(a *HeapDumpAnalyzer, rootObjectIds []uint64) {
	r.logger.Debug("--- ScanRoot ---: %v", len(rootObjectIds))
	seen := NewSeen()
	for _, rootObjectId := range rootObjectIds {
		r.logger.Debug("rootObjectId=%v", rootObjectId)
		r.scan(rootObjectId, 0, rootObjectId, a, seen)
	}
	r.logger.Debug("--- /ScanRoot --- %v %v", len(r.nearestGcRoot), len(r.gcRootDistance))
}

func (r RootScanner) scan(rootObjectId uint64, distance int, objectId uint64, a *HeapDumpAnalyzer, seen *Seen) {
	if objectId == 0 {
		return // NULL
	}
	if seen.HasKey(objectId) {
		return
	}

	a.logger.Indent()
	defer a.logger.Dedent()

	seen.Add(objectId)

	r.RegisterRoot(rootObjectId, objectId, distance)

	instanceDump := a.objectId2instanceDump[objectId]
	if instanceDump != nil {
		r.logger.Debug("instance dump = %v", objectId)

		classDump := a.classObjectId2classDump[instanceDump.ClassObjectId]
		values := instanceDump.GetValues()
		idx := 0

		for {
			for _, instanceField := range classDump.InstanceFields {
				if instanceField.Type == hprofdata.HProfValueType_OBJECT {
					// TODO 32bit support
					r.logger.Trace("instance field = %v.%v", instanceDump.ObjectId,
						a.nameId2string[instanceField.NameId])
					objectIdBytes := values[idx : idx+8]
					objectId := binary.BigEndian.Uint64(objectIdBytes)
					r.scan(rootObjectId, distance+1, objectId, a, seen)
					idx += 8
				} else {
					idx += parser.ValueSize[instanceField.Type]
				}
			}
			classDump = a.classObjectId2classDump[classDump.SuperClassObjectId]
			if classDump == nil {
				break
			}
		}
		return
	}

	classDump := a.classObjectId2classDump[objectId]
	if classDump != nil {
		// scan super
		r.logger.Debug("class dump = %v", objectId)

		idx := 0
		for _, field := range classDump.StaticFields {
			if field.Type == hprofdata.HProfValueType_OBJECT {
				v := field.GetValue()
				r.scan(rootObjectId, distance+1, v, a, seen)
				idx += 8
			} else {
				idx += parser.ValueSize[field.Type]
			}
		}

		// scan super class
		super := a.classObjectId2classDump[classDump.SuperClassObjectId]
		if super != nil {
			r.scan(rootObjectId, distance+1, super.ClassObjectId, a, seen)
		}
		return
	}

	// object array
	objectArrayDump := a.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		r.logger.Debug("object array = %v", objectId)
		for _, objectId := range objectArrayDump.ElementObjectIds {
			r.scan(rootObjectId, distance+1, objectId, a, seen)
		}
		return
	}

	// primitive array
	primitiveArrayDump := a.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		r.logger.Debug("primitive array = %v", objectId)
		return
	}

	log.Fatalf("SHOULD NOT REACH HERE: %v pa=%v oa=%v id=%v cd=%v",
		objectId,
		a.arrayObjectId2primitiveArrayDump[objectId],
		a.arrayObjectId2objectArrayDump[objectId],
		a.objectId2instanceDump[objectId],
		a.classObjectId2classDump[objectId])
}

func (r RootScanner) RegisterRoot(rootObjectId uint64, targetObjectId uint64, distance int) {
	if currentDistance, ok := r.gcRootDistance[targetObjectId]; !ok {
		r.logger.Trace("RegisterRoot: rootObjectId=%v targetObjectId=%v distance=%v currentDistance=%v",
			rootObjectId, targetObjectId, distance, currentDistance)
		r.setRegisterRoot(rootObjectId, targetObjectId, distance)
	} else {
		r.logger.Trace("RegisterRoot: rootObjectId=%v targetObjectId=%v distance=%v currentDistance=%v",
			rootObjectId, targetObjectId, distance, currentDistance)
		if currentDistance > distance {
			r.setRegisterRoot(rootObjectId, targetObjectId, distance)
		}
	}
}

func (r RootScanner) setRegisterRoot(rootObjectId uint64, targetObjectId uint64, distance int) {
	r.gcRootDistance[targetObjectId] = distance
	r.nearestGcRoot[targetObjectId] = rootObjectId
}

func (r RootScanner) GetNearestGcRoot(objectId uint64) uint64 {
	return r.nearestGcRoot[objectId]
}

func (r RootScanner) ScanAll(analyzer *HeapDumpAnalyzer) {
	r.ScanRoot(analyzer, keys(analyzer.rootJniGlobals))
	r.ScanRoot(analyzer, keys(analyzer.rootJniLocal))
	r.ScanRoot(analyzer, keys(analyzer.rootJavaFrame))
	r.ScanRoot(analyzer, keys(analyzer.rootStickyClass))
	r.ScanRoot(analyzer, keys(analyzer.rootThreadObj))
	r.ScanRoot(analyzer, keys(analyzer.rootMonitorUsed))
}
