package main

import (
	"golang.org/x/text/message"
	"io/ioutil"
	"os"
	"sort"
)

type HeapDumpAnalyzer struct {
	logger                 *Logger
	hprof                  *HProf // TODO deprecate this.
	softSizeCalculator     *SoftSizeCalculator
	retainedSizeCalculator *RetainedSizeCalculator
}

func NewHeapDumpAnalyzer(logger *Logger) (*HeapDumpAnalyzer, error) {
	m := new(HeapDumpAnalyzer)
	m.logger = logger

	// todo: clean up tempdir.
	indexPath, err := ioutil.TempDir(os.TempDir(), "hprof")
	if err != nil {
		return nil, err
	}

	m.logger.Info("Opening index path: %v", indexPath)

	m.hprof, _ = NewHProf(logger, indexPath)
	m.softSizeCalculator = NewSoftSizeCalculator(logger)
	m.retainedSizeCalculator = NewRetainedSizeCalculator(logger)
	return m, nil
}

func (a HeapDumpAnalyzer) ReadFile(heapFilePath string) error {
	return a.hprof.ReadFile(heapFilePath)
}

func (a HeapDumpAnalyzer) DumpInclusiveRanking(rootScanner *RootScanner) error {
	a.logger.Debug("DumpInclusiveRanking")
	var classObjectIds []uint64
	for k := range a.hprof.classObjectId2objectIds {
		classObjectIds = append(classObjectIds, k)
	}

	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectIds[i] < classObjectIds[j]
	})

	var classObjectId2objectSize = map[uint64]uint64{}
	classObjectId2objectCount := make(map[uint64]int)
	for _, classObjectId := range classObjectIds {
		objectIds := a.hprof.classObjectId2objectIds[classObjectId]
		name, err := a.hprof.GetClassNameByClassObjectId(classObjectId)
		if err != nil {
			return err
		}

		for _, objectId := range objectIds {
			a.logger.Debug("Starting scan %v(classObjectId=%v, objectId=%v)\n",
				name, classObjectId, objectId)

			size, err := a.GetRetainedSize(objectId, rootScanner)
			if err != nil {
				return err
			}
			classObjectId2objectSize[classObjectId] += size

			a.logger.Debug("Finished scan %v(classObjectId=%v, objectId=%v) size=%v\n",
				name, classObjectId, objectId, size)
		}
		classObjectId2objectCount[classObjectId] = len(objectIds)
	}

	// sort by retained size
	sort.Slice(classObjectIds, func(i, j int) bool {
		return classObjectId2objectSize[classObjectIds[i]] < classObjectId2objectSize[classObjectIds[j]]
	})

	// print result
	p := message.NewPrinter(message.MatchLanguage("en"))
	for _, classObjectId := range classObjectIds {
		name, err := a.hprof.GetClassNameByClassObjectId(classObjectId)
		if err != nil {
			return err
		}
		n, err := a.softSizeCalculator.CalcSoftSizeByClassObjectId(a.hprof, classObjectId)
		if err != nil {
			return err
		}
		a.logger.Info(p.Sprintf("shallowSize=%11d retainedSize=%11d(count=%11d)= %s",
			n,
			classObjectId2objectSize[classObjectId],
			classObjectId2objectCount[classObjectId],
			name))
	}

	return nil
}

func (a HeapDumpAnalyzer) GetRetainedSize(objectId uint64, rootScanner *RootScanner) (uint64, error) {
	return a.retainedSizeCalculator.GetRetainedSize(a.hprof, rootScanner, objectId)
}

func (a HeapDumpAnalyzer) CalculateRetainedSizeOfInstancesByName(targetName string, rootScanner *RootScanner) (map[uint64]uint64, error) {
	objectID2size := make(map[uint64]uint64)

	for classObjectId, objectIds := range a.hprof.classObjectId2objectIds {
		name, err := a.hprof.GetClassNameByClassObjectId(classObjectId)
		if err != nil {
			return nil, err
		}
		if name == targetName {
			for _, objectId := range objectIds {
				a.logger.Debug("**** Scanning %v objectId=%v", targetName, objectId)
				size, err := a.GetRetainedSize(objectId, rootScanner)
				if err != nil {
					return nil, err
				}
				objectID2size[objectId] = size
				a.logger.Debug("**** Scanned %v\n\n", size)
			}
			break
		}
	}

	return objectID2size, nil
}
