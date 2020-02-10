package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"log"
)

type RootScanner struct {
	parents map[uint64]uint64 // objectId -> count
	logger  *Logger
}

func NewRootScanner(logger *Logger) *RootScanner {
	m := new(RootScanner)
	m.parents = make(map[uint64]uint64)
	m.logger = logger
	return m
}

func (r RootScanner) ScanRoot(a *HeapDumpAnalyzer, rootObjectIds []uint64) error {
	r.logger.Debug("--- ScanRoot ---: %v", len(rootObjectIds))
	seen := NewSeen()
	for _, rootObjectId := range rootObjectIds {
		r.logger.Debug("rootObjectId=%v", rootObjectId)
		err := r.scan(rootObjectId, a, seen)
		if err != nil {
			return err
		}
	}
	r.logger.Debug("--- /ScanRoot ---")
	return nil
}

func (r RootScanner) scan(objectId uint64, a *HeapDumpAnalyzer, seen *Seen) error {
	if objectId == 0 {
		return nil // NULL
	}
	if seen.HasKey(objectId) {
		return nil
	}

	a.logger.Indent()
	defer a.logger.Dedent()

	seen.Add(objectId)

	instanceDump := a.hprof.objectId2instanceDump[objectId]
	if instanceDump != nil {
		r.logger.Debug("instance dump = %v", objectId)

		classDump, err := a.hprof.GetClassDumpByClassObjectId(instanceDump.ClassObjectId)
		if err != nil {
			return err
		}
		values := instanceDump.GetValues()
		idx := 0

		for {
			for _, instanceField := range classDump.InstanceFields {
				if instanceField.Type == hprofdata.HProfValueType_OBJECT {
					// TODO 32bit support
					r.logger.Trace("instance field = %v", instanceDump.ObjectId)
					objectIdBytes := values[idx : idx+8]
					childObjectId := binary.BigEndian.Uint64(objectIdBytes)
					r.RegisterParent(objectId, childObjectId)
					err := r.scan(childObjectId, a, seen)
					if err != nil {
						return err
					}
					idx += 8
				} else {
					idx += parser.ValueSize[instanceField.Type]
				}
			}
			classDump, err = a.hprof.GetClassDumpByClassObjectId(classDump.SuperClassObjectId)
			if err != nil {
				return err
			}
			if classDump == nil {
				break
			}
		}
		return nil
	}

	classDump, err := a.hprof.GetClassDumpByClassObjectId(objectId)
	if err != nil {
		return err
	}
	if classDump != nil {
		// scan super
		r.logger.Debug("class dump = %v", objectId)

		idx := 0
		for _, field := range classDump.StaticFields {
			if field.Type == hprofdata.HProfValueType_OBJECT {
				childObjectId := field.GetValue()
				r.RegisterParent(objectId, childObjectId)
				err := r.scan(childObjectId, a, seen)
				if err != nil {
					return err
				}
				idx += 8
			} else {
				idx += parser.ValueSize[field.Type]
			}
		}

		// scan super class
		super, err := a.hprof.GetClassDumpByClassObjectId(classDump.SuperClassObjectId)
		if err != nil {
			return err
		}
		if super != nil {
			r.RegisterParent(objectId, super.ClassObjectId)
			err := r.scan(super.ClassObjectId, a, seen)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// object array
	objectArrayDump := a.hprof.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		r.logger.Debug("object array = %v", objectId)
		for _, childObjectId := range objectArrayDump.ElementObjectIds {
			r.RegisterParent(objectId, childObjectId)
			err := r.scan(childObjectId, a, seen)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// primitive array
	primitiveArrayDump := a.hprof.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		r.logger.Debug("primitive array = %v", objectId)
		return nil
	}

	log.Fatalf("SHOULD NOT REACH HERE: %v pa=%v oa=%v id=%v",
		objectId,
		a.hprof.arrayObjectId2primitiveArrayDump[objectId],
		a.hprof.arrayObjectId2objectArrayDump[objectId],
		a.hprof.objectId2instanceDump[objectId])
	return nil
}

func (r RootScanner) RegisterParent(parentObjectId uint64, childObjectId uint64) {
	currentParentId, ok := r.parents[childObjectId]
	if ok {
		if currentParentId == 0 {
			// ignore. already duplicated.
		} else {
			if parentObjectId != currentParentId {
				// reference from another root.
				r.parents[childObjectId] = 0
			}
		}
	} else {
		r.parents[childObjectId] = parentObjectId
		r.logger.Trace("RegisterParent: parentObjectId=%v childObjectId=%v distance=%v currentDistance=%v",
			parentObjectId, childObjectId)
	}
}

/**
Returns true if the `objectId` is referenced from only the parent object.
*/
func (r RootScanner) IsRetained(parentObjectId uint64, childObjectId uint64) bool {
	theParentObjectId, ok := r.parents[childObjectId]
	if !ok {
		return false
	}
	return theParentObjectId == parentObjectId
}

func (r RootScanner) ScanAll(analyzer *HeapDumpAnalyzer) error {
	r.logger.Info("Scanning retained root")
	for _, f := range [][]uint64{
		keys(analyzer.hprof.rootJniGlobals),
		keys(analyzer.hprof.rootJniLocal),
		keys(analyzer.hprof.rootJavaFrame),
		keys(analyzer.hprof.rootStickyClass),
		keys(analyzer.hprof.rootThreadObj),
		keys(analyzer.hprof.rootMonitorUsed),
	} {
		err := r.ScanRoot(analyzer, f)
		if err != nil {
			return err
		}
	}
	return nil
}
