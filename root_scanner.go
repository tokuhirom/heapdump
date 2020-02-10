package main

import (
	"encoding/binary"
	"fmt"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"github.com/syndtr/goleveldb/leveldb/errors"
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
			return fmt.Errorf("ScanRoot: %v", err)
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
		classDump, err := a.hprof.GetClassObjectByClassObjectId(instanceDump.ClassObjectId)
		if err != nil {
			return fmt.Errorf("cannot get class object from instance: %v", err)
		}
		if r.logger.IsDebugEnabled() {
			name, err := a.hprof.GetClassNameByClassObjectId(classDump.ClassObjectId)
			if err != nil {
				return fmt.Errorf("err: %v", err)
			}
			r.logger.Debug("instance dump = %v(%v)", objectId, name)
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
						return fmt.Errorf("scan failed(%v): %v",
							instanceDump.ObjectId, err)
					}
					idx += 8
				} else {
					idx += parser.ValueSize[instanceField.Type]
				}
			}
			if classDump.SuperClassObjectId == 0 {
				break
			}
			classDump, err = a.hprof.GetClassObjectByClassObjectId(classDump.SuperClassObjectId)
			if err != nil {
				return fmt.Errorf("cannot get class object in scan: %v", err)
			}
			if classDump == nil {
				break
			}
		}
		return nil
	}

	classDump, err := a.hprof.GetClassObjectByClassObjectId(objectId)
	if err != nil && errors.ErrNotFound != err {
		return fmt.Errorf("in scan: %v", err)
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
		super, err := a.hprof.GetClassObjectByClassObjectId(classDump.SuperClassObjectId)
		if err != nil && err != errors.ErrNotFound {
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

	// TODO: refactoring
	err := r.ScanRoot(analyzer, keys(analyzer.hprof.rootJniGlobals))
	if err != nil {
		return err
	}
	err = r.ScanRoot(analyzer, keys(analyzer.hprof.rootJniLocal))
	if err != nil {
		return err
	}
	err = r.ScanRoot(analyzer, keys(analyzer.hprof.rootJavaFrame))
	if err != nil {
		return err
	}
	err = r.ScanRoot(analyzer, keys(analyzer.hprof.rootStickyClass))
	if err != nil {
		return err
	}
	err = r.ScanRoot(analyzer, keys(analyzer.hprof.rootThreadObj))
	if err != nil {
		return err
	}
	err = r.ScanRoot(analyzer, keys(analyzer.hprof.rootMonitorUsed))
	if err != nil {
		return err
	}
	return nil
}
