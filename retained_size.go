package main

import (
	"encoding/binary"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"log"
)

type RetainedSizeCalculator struct {
	logger    *Logger
	sizeCache map[uint64]uint64
}

func NewRetainedSizeCalculator(logger *Logger) *RetainedSizeCalculator {
	m := new(RetainedSizeCalculator)
	m.logger = logger
	m.sizeCache = make(map[uint64]uint64)
	return m
}

func (a RetainedSizeCalculator) GetRetainedSize(hprof *HProf, rootScanner *RootScanner, objectId uint64) (uint64, error) {
	seen := NewSeen()
	return a.retainedSizeInstance(hprof, objectId, seen, rootScanner)
}

func (a RetainedSizeCalculator) retainedSizeInstance(hprof *HProf, objectId uint64, seen *Seen, rootScanner *RootScanner) (uint64, error) {
	if seen == nil {
		panic("Missing seen")
	}
	if seen.HasKey(objectId) { // recursive counting.
		a.logger.Debug("Recursive counting occurred: %v", objectId)
		return 0, nil
	}

	if size, ok := a.getSizeCache(objectId); ok {
		return size, nil
	}

	seen.Add(objectId)
	//if seen.Size() > 3000 {
	//	panic("Too much seen.")
	//}

	a.logger.Indent()
	defer a.logger.Dedent()

	instanceDump := hprof.objectId2instanceDump[objectId]
	if instanceDump != nil {
		if a.logger.IsDebugEnabled() {
			name, err := hprof.GetClassNameByClassObjectId(instanceDump.ClassObjectId)
			if err != nil {
				return 0, err
			}

			a.logger.Debug("retainedSizeInstance(%v) objectId=%d seen=%v",
				name, objectId, seen.Size())
		}
		return a.calcObjectSize(hprof, instanceDump, objectId, seen, rootScanner)
	}

	a.logger.Debug("retainedSizeInstance() objectId=%d seen=%v",
		objectId,
		seen.Size())

	objectArrayDump := hprof.arrayObjectId2objectArrayDump[objectId]
	if objectArrayDump != nil {
		return a.calcObjectArraySize(hprof, objectArrayDump, seen, rootScanner), nil
	}

	primitiveArrayDump := hprof.arrayObjectId2primitiveArrayDump[objectId]
	if primitiveArrayDump != nil {
		return a.calcPrimitiveArraySize(primitiveArrayDump), nil
	}

	classDump, err := hprof.GetClassDumpByClassObjectId(objectId)
	if err != nil {
		return 0, err
	}
	if classDump != nil {
		return a.calcClassSize(hprof, classDump, seen, rootScanner), nil
	}

	log.Fatalf(
		"[ERROR] Unknown instance: objectId=%v instanceDump=%v primArray=%v objArray=%v",
		objectId,
		instanceDump,
		hprof.arrayObjectId2primitiveArrayDump[objectId],
		hprof.arrayObjectId2objectArrayDump[objectId])
	return 0, nil // should not reach here
}

func (a RetainedSizeCalculator) getSizeCache(objectId uint64) (uint64, bool) {
	size, ok := a.sizeCache[objectId]
	return size, ok
}

func (a RetainedSizeCalculator) setSizeCache(objectId uint64, size uint64) {
	a.sizeCache[objectId] = size
}

func (a RetainedSizeCalculator) calcObjectSize(
	hprof *HProf,
	instanceDump *hprofdata.HProfInstanceDump,
	objectId uint64,
	seen *Seen,
	rootScanner *RootScanner) (uint64, error) {
	classDump, err := hprof.GetClassDumpByClassObjectId(instanceDump.ClassObjectId)
	if err != nil {
		return 0, err
	}
	a.logger.Debug("calcObjectSize oid=%d",
		objectId)

	// instance field を舐めてサイズを計算する。super があればそこも全てみる。

	// 全てのインスタンスは 16 byte かかるっぽい。
	// https://weekly-geekly.github.io/articles/447848/index.html
	// On a 64-bit jvm, the object header consists of 16 bytes. Arrays are additionally 4 bytes.

	size := uint64(16 + len(instanceDump.GetValues()))
	values := instanceDump.GetValues()
	// 読み込んだバイト数, サイズ
	for {
		name, err := hprof.GetClassNameByClassObjectId(classDump.ClassObjectId)
		if err != nil {
			return 0, err
		}
		a.logger.Trace("scan instance: name=%v",
			name)

		nIdx, nSize, err := a.scanInstance(hprof, classDump.InstanceFields, values, seen, name, objectId, rootScanner)
		if err != nil {
			return 0, err
		}
		size += nSize
		a.logger.Trace("nSize=%v (%v)", nSize, name)
		values = values[nIdx:]
		if classDump.SuperClassObjectId == 0 {
			break
		} else {
			classDump, err = hprof.GetClassDumpByClassObjectId(classDump.SuperClassObjectId)
			if err != nil {
				return 0, err
			}
		}
	}

	a.logger.Debug("/calcObjectSize oid=%d size=%v seen=%v",
		objectId, size, seen)

	a.setSizeCache(objectId, size)
	return size, nil
}

func (a RetainedSizeCalculator) scanInstance(
	hprof *HProf,
	fields []*hprofdata.HProfClassDump_InstanceField,
	values []byte,
	seen *Seen,
	className string,
	parentObjectId uint64,
	rootScanner *RootScanner) (int, uint64, error) {

	size := uint64(0)
	idx := 0

	for _, field := range fields {
		if field.Type == hprofdata.HProfValueType_OBJECT {
			// TODO 32bit support(特にやる気なし)
			objectIdBytes := values[idx : idx+8]
			childObjectId := binary.BigEndian.Uint64(objectIdBytes)
			if childObjectId == 0 {
				// the field contains null
				a.logger.Trace("object field: className=%v oid=NULL",
					className)
			} else {
				a.logger.Trace("start:: object field: className=%v oid=%v",
					className,
					childObjectId)
				if rootScanner.IsRetained(parentObjectId, childObjectId) {
					n, err := a.retainedSizeInstance(hprof, childObjectId, seen, rootScanner)
					if err != nil {
						return 0, 0, err
					}
					a.logger.Trace("finished:: object field: className=%v oid=%v, size=%v",
						className,
						childObjectId, n)
					size += n
				} else {
					a.logger.Trace("IGNORE!!:: object field: className=%v oid=%v",
						className,
						childObjectId)
				}
			}
			idx += 8
		} else {
			a.logger.Trace("Primitive Field")
			idx += parser.ValueSize[field.Type]
		}
	}

	return idx, size, nil
}

func (a RetainedSizeCalculator) calcObjectArraySize(hprof *HProf, dump *hprofdata.HProfObjectArrayDump, seen *Seen,
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
				s, _ := a.retainedSizeInstance(hprof, objectId, seen, rootScanner)
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

func (a RetainedSizeCalculator) calcPrimitiveArraySize(dump *hprofdata.HProfPrimitiveArrayDump) uint64 {
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

func (a RetainedSizeCalculator) calcClassSize(hprof *HProf, dump *hprofdata.HProfClassDump, seen *Seen, rootScanner *RootScanner) uint64 {
	a.logger.Debug("calcClassSize: %v",
		dump.ClassObjectId)

	totalSize := uint64(0)
	for _, field := range dump.StaticFields {
		if field.Type == hprofdata.HProfValueType_OBJECT {
			childObjectId := field.Value
			totalSize += 8
			if childObjectId != 0 {
				if rootScanner.IsRetained(dump.ClassObjectId, childObjectId) {
					size, _ := a.retainedSizeInstance(hprof, childObjectId, seen, rootScanner)
					totalSize += size
				}
			}
		} else {
			totalSize += uint64(parser.ValueSize[field.Type])
		}
	}
	return totalSize
}
