package main

type RCObject interface {
	AddRef()
	Release()
	RefCount() int64
	MarkForCycle()
	ClearCycleMark()
	IsCycleMarked() bool
	CycleColor() CycleColor
	SetCycleColor(c CycleColor)
	IsBuffered() bool
	SetBuffered(b bool)
	Traverse(fn func(RCObject))
	Dispose()
}

type CycleColor uint8

const (
	ColorBlack CycleColor = iota
	ColorGray
	ColorWhite
	ColorPurple
)

type RCHeader struct {
	refCount   int64
	cycleColor CycleColor
	cycleMark  bool
	buffered   bool
}

func NewRCHeader() RCHeader {
	return RCHeader{
		refCount:   1,
		cycleColor: ColorBlack,
	}
}

func (h *RCHeader) RefCount() int64 {
	return h.refCount
}

func (h *RCHeader) IncRef() {
	h.refCount++
}

func (h *RCHeader) DecRef() int64 {
	if h.refCount > 0 {
		h.refCount--
	}
	return h.refCount
}

func (h *RCHeader) MarkForCycle() {
	h.cycleMark = true
}

func (h *RCHeader) ClearCycleMark() {
	h.cycleMark = false
}

func (h *RCHeader) IsCycleMarked() bool {
	return h.cycleMark
}

func (h *RCHeader) CycleColor() CycleColor {
	return h.cycleColor
}

func (h *RCHeader) SetCycleColor(c CycleColor) {
	h.cycleColor = c
}

func (h *RCHeader) IsBuffered() bool {
	return h.buffered
}

func (h *RCHeader) SetBuffered(b bool) {
	h.buffered = b
}

type CycleCollector struct {
	heap        *RCHeaderHeap
	purpleQueue []RCObject
}

type RCHeaderHeap struct {
	objects []RCObject
}

func NewRCHeaderHeap() *RCHeaderHeap {
	return &RCHeaderHeap{}
}

func (h *RCHeaderHeap) Register(obj RCObject) {
	if obj == nil {
		return
	}
	h.objects = append(h.objects, obj)
}

func (h *RCHeaderHeap) Unregister(obj RCObject) {
	if obj == nil {
		return
	}
	for i, cur := range h.objects {
		if cur == obj {
			last := len(h.objects) - 1
			h.objects[i] = h.objects[last]
			h.objects[last] = nil
			h.objects = h.objects[:last]
			return
		}
	}
}

func NewCycleCollector(heap *RCHeaderHeap) *CycleCollector {
	return &CycleCollector{
		heap: heap,
	}
}

func (cc *CycleCollector) PossibleRoot(obj RCObject) {
	if obj == nil || obj.RefCount() <= 0 {
		return
	}
	if obj.CycleColor() != ColorPurple {
		obj.SetCycleColor(ColorPurple)
		if !obj.IsBuffered() {
			obj.SetBuffered(true)
			cc.purpleQueue = append(cc.purpleQueue, obj)
		}
	}
}

func (cc *CycleCollector) Collect() {
	cc.markRoots()
	cc.scanRoots()
	cc.collectRoots()
}

func (cc *CycleCollector) markRoots() {
	var nextQueue []RCObject
	for _, obj := range cc.purpleQueue {
		if obj == nil {
			continue
		}
		if obj.CycleColor() == ColorPurple && obj.RefCount() > 0 {
			cc.markGray(obj)
			nextQueue = append(nextQueue, obj)
		} else {
			obj.SetBuffered(false)
			if obj.RefCount() <= 0 {
				obj.Dispose()
			}
		}
	}
	cc.purpleQueue = nextQueue
}

func (cc *CycleCollector) markGray(obj RCObject) {
	if obj == nil {
		return
	}
	if obj.CycleColor() != ColorGray {
		obj.SetCycleColor(ColorGray)
		obj.Traverse(func(child RCObject) {
			if child != nil && child.RefCount() > 0 {
				if listObj, ok := child.(*VMList); ok {
					listObj.header.DecRef()
				} else if structObj, ok := child.(*VMStruct); ok {
					structObj.header.DecRef()
				} else if closObj, ok := child.(*VMClosure); ok {
					closObj.header.DecRef()
				}
				cc.markGray(child)
			}
		})
	}
}

func (cc *CycleCollector) scanRoots() {
	for _, obj := range cc.purpleQueue {
		if obj != nil {
			cc.scan(obj)
		}
	}
}

func (cc *CycleCollector) scan(obj RCObject) {
	if obj == nil {
		return
	}
	if obj.CycleColor() == ColorGray {
		if obj.RefCount() > 0 {
			cc.scanBlack(obj)
		} else {
			obj.SetCycleColor(ColorWhite)
			obj.Traverse(func(child RCObject) {
				cc.scan(child)
			})
		}
	}
}

func (cc *CycleCollector) scanBlack(obj RCObject) {
	if obj == nil {
		return
	}
	obj.SetCycleColor(ColorBlack)
	obj.Traverse(func(child RCObject) {
		if child != nil {
			if listObj, ok := child.(*VMList); ok {
				listObj.header.IncRef()
			} else if structObj, ok := child.(*VMStruct); ok {
				structObj.header.IncRef()
			} else if closObj, ok := child.(*VMClosure); ok {
				closObj.header.IncRef()
			}
			if child.CycleColor() != ColorBlack {
				cc.scanBlack(child)
			}
		}
	})
}

func (cc *CycleCollector) collectRoots() {
	for _, obj := range cc.purpleQueue {
		if obj != nil {
			obj.SetBuffered(false)
			cc.collectWhite(obj)
		}
	}
	cc.purpleQueue = nil
}

func (cc *CycleCollector) collectWhite(obj RCObject) {
	if obj == nil {
		return
	}
	if obj.CycleColor() == ColorWhite && !obj.IsBuffered() {
		obj.SetCycleColor(ColorBlack)
		obj.Traverse(func(child RCObject) {
			cc.collectWhite(child)
		})
		if cc.heap != nil {
			cc.heap.Unregister(obj)
		}
		obj.Dispose()
	}
}
