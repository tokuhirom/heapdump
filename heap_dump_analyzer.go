package main

import (
	"golang.org/x/text/message"
	"sort"
)

type HeapDumpAnalyzer struct {
	logger                 *Logger
	hprof                  *HProf
	softSizeCalculator     *SoftSizeCalculator
	retainedSizeCalculator *RetainedSizeCalculator
}

func NewHeapDumpAnalyzer(logger *Logger) *HeapDumpAnalyzer {
	m := new(HeapDumpAnalyzer)
	m.logger = logger
	m.hprof = NewHProf(logger)
	m.softSizeCalculator = NewSoftSizeCalculator(logger)
	m.retainedSizeCalculator = NewRetainedSizeCalculator(logger)
	return m
}

func (a HeapDumpAnalyzer) ReadFile(heapFilePath string) error {
	return a.hprof.ReadFile(heapFilePath)
}

func (a HeapDumpAnalyzer) DumpInclusiveRanking(rootScanner *RootScanner) {
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
		classNameId := a.hprof.classObjectId2classNameId[classObjectId]
		name := a.hprof.nameId2string[classNameId]

		for _, objectId := range objectIds {
			a.logger.Debug("Starting scan %v(classObjectId=%v, objectId=%v)\n",
				name, classObjectId, objectId)

			size := a.GetRetainedSize(objectId, rootScanner)
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
		classNameId := a.hprof.classObjectId2classNameId[classObjectId]
		name := a.hprof.nameId2string[classNameId]
		a.logger.Info(p.Sprintf("shallowSize=%10d retainedSize=%10d(count=%5d)= %s",
			a.softSizeCalculator.CalcSoftSizeByClassObjectId(a.hprof, classObjectId),
			classObjectId2objectSize[classObjectId],
			classObjectId2objectCount[classObjectId],
			name))
	}
}

func (a HeapDumpAnalyzer) GetRetainedSize(objectId uint64, rootScanner *RootScanner) uint64 {
	return a.retainedSizeCalculator.GetRetainedSize(a.hprof, rootScanner, objectId)
}

func (a HeapDumpAnalyzer) CalculateRetainedSizeOfInstancesByName(targetName string, rootScanner *RootScanner) map[uint64]uint64 {
	objectID2size := make(map[uint64]uint64)

	for classObjectId, objectIds := range a.hprof.classObjectId2objectIds {
		name := a.hprof.nameId2string[a.hprof.classObjectId2classNameId[classObjectId]]
		if name == targetName {
			for _, objectId := range objectIds {
				a.logger.Debug("**** Scanning %v objectId=%v", targetName, objectId)
				size := a.GetRetainedSize(objectId, rootScanner)
				objectID2size[objectId] = size
				a.logger.Debug("**** Scanned %v\n\n", size)
			}
			break
		}
	}

	return objectID2size
}
