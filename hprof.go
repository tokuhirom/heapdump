package main

import (
	"encoding/binary"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/google/hprof-parser/hprofdata"
	"github.com/google/hprof-parser/parser"
	"github.com/syndtr/goleveldb/leveldb"
	"io"
	"os"
	"strconv"
)

const (
	keyPrefixString                    = "string-"
	keyPrefixClassObjectId2ClassNameId = "coid2cnid-"
	keyPrefixFrame                     = "frame-"
	keyPrefixTrace                     = "trace-"
	keyPrefixClass                     = "class-"
	keyPrefixInstance                  = "instance-"
	keyPrefixObjectArray               = "objectarray-"
	keyPrefixPrimitiveArray            = "primitivearray-"
	keyPrefixRootJNIGlobal             = "rootjniglobal-"
	keyPrefixRootJNILocal              = "rootjnilocal-"
	keyPrefixRootJavaFrame             = "rootjavaframe-"
	keyPrefixRootStickyClass           = "rootstickyclass-"
	keyPrefixRootThreadObj             = "rootthreadobj-"
)

type HProf struct {
	logger *Logger

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
	db              *leveldb.DB
}

func NewHProf(logger *Logger, indexFilePath string) (*HProf, error) {
	m := new(HProf)

	m.logger = logger

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

	db, err := leveldb.OpenFile(indexFilePath, nil)
	if err != nil {
		return nil, err
	}
	m.db = db

	return m, nil
}

func (h HProf) Close() error {
	return h.db.Close()
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

	batch := new(leveldb.Batch)

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

		err = h.addRecord(record, batch)
		if err != nil {
			return err
		}
		if batch.Len() > 100000 {
			if err := h.db.Write(batch, nil); err != nil {
				return err
			}
			batch.Reset()
		}
	}

	// At last, write the mtime of the hprof into the DB.
	mtime, err := getMtimeInString(heapFilePath)
	if err != nil {
		return err
	}
	batch.Put([]byte("hprof_mtime"), []byte(mtime))

	if err := h.db.Write(batch, nil); err != nil {
		return err
	}

	return nil
}

func getMtimeInString(fileName string) (string, error) {
	fi, err := os.Stat(fileName)
	if err != nil {
		return "", err
	}
	mtime := fi.ModTime()
	return strconv.FormatUint(uint64(mtime.Unix()), 36), nil
}

func createKey(prefix string, id uint64) []byte {
	return []byte(prefix + strconv.FormatUint(id, 16))
}

func writeRecord(batch *leveldb.Batch, prefix string, id uint64, record interface{}) error {
	m := record.(proto.Message)
	bs, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	batch.Put(createKey(prefix, id), bs)
	return nil
}

func (h HProf) addRecord(record interface{}, batch *leveldb.Batch) error {
	switch o := record.(type) {
	case *hprofdata.HProfRecordUTF8:
		batch.Put(createKey(keyPrefixString, o.GetNameId()), o.GetName())
	case *hprofdata.HProfRecordLoadClass:
		buf := make([]byte, binary.MaxVarintLen64)
		n := binary.PutUvarint(buf, o.GetClassNameId())
		batch.Put(createKey(keyPrefixClassObjectId2ClassNameId, o.GetClassObjectId()), buf[:n])
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
	return nil
}

func (h HProf) GetStringByNameId(id uint64) (string, error) {
	bytes, err := h.db.Get(createKey(keyPrefixString, id), nil)
	if err != nil {
		return "", fmt.Errorf("cannot read string by id=%v, key=%v: %v",
			id, createKey(keyPrefixString, id), err)
	}
	return string(bytes), nil
}

func (h HProf) GetClassNameIdByClassObjectId(id uint64) (uint64, error) {
	bytes, err := h.db.Get(createKey(keyPrefixClassObjectId2ClassNameId, id), nil)
	if err != nil {
		return 0, fmt.Errorf("cannot read string by id=%v, key=%v: %v",
			id, createKey(keyPrefixString, id), err)
	}
	x, n := binary.Uvarint(bytes)
	if n != len(bytes) {
		return 0, fmt.Errorf("uvarint did not consume all of in")
	}
	return x, nil
}

func (h HProf) GetClassNameByClassObjectId(classObjectId uint64) (string, error) {
	classNameId, err := h.GetClassNameIdByClassObjectId(classObjectId)
	if err != nil {
		return "", err
	}
	return h.GetStringByNameId(classNameId)
}
