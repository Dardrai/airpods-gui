package autopause

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The system media transport controls are a WinRT API. Go has no projection of it
// without extra tooling, but WinRT objects are plain vtables, so the few calls needed
// are made by slot. Slots count from the three IUnknown and three IInspectable entries.
const (
	slotRelease        = 2
	slotQueryInterface = 0

	slotRequestAsync      = 6 // ...SessionManagerStatics
	slotGetCurrentSession = 6 // ...SessionManager
	slotGetPlaybackInfo   = 9 // ...Session
	slotTryPlayAsync      = 10
	slotTryPauseAsync     = 11
	slotPlaybackStatus    = 7 // ...SessionPlaybackInfo
	slotGetResults        = 8 // IAsyncOperation
	slotAsyncStatus       = 7 // IAsyncInfo
	slotAsyncCancel       = 9

	statusPlaying = 4
	statusPaused  = 5

	asyncStarted   = 0
	asyncCompleted = 1

	asyncLimit  = 2 * time.Second
	asyncStep   = 10 * time.Millisecond
	pauseSettle = 1500 * time.Millisecond
)

const managerClass = "Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager"

var (
	managerStaticsIID = windows.GUID{Data1: 0x2050c4ee, Data2: 0x11a0, Data3: 0x57de,
		Data4: [8]byte{0xae, 0xd7, 0xc9, 0x7c, 0x70, 0x33, 0x82, 0x45}}
	asyncInfoIID = windows.GUID{Data1: 0x00000036, Data2: 0, Data3: 0,
		Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

	combase                = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize       = combase.NewProc("RoInitialize")
	procRoGetFactory       = combase.NewProc("RoGetActivationFactory")
	procWindowsCreate      = combase.NewProc("WindowsCreateString")
	procWindowsDelete      = combase.NewProc("WindowsDeleteString")
	errNoTransportControls = errors.New("media controls are not available")
)

type object = unsafe.Pointer

func method(obj object, slot int) uintptr {
	table := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(table, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
}

func hresult(r uintptr) error {
	if int32(uint32(r)) < 0 {
		return fmt.Errorf("media controls: 0x%08x", uint32(r))
	}
	return nil
}

func release(obj object) {
	if obj != nil {
		syscall.SyscallN(method(obj, slotRelease), uintptr(obj))
	}
}

// getObject calls a method whose only argument is an object out parameter.
func getObject(obj object, slot int) (object, error) {
	var out object
	r, _, _ := syscall.SyscallN(method(obj, slot), uintptr(obj), uintptr(unsafe.Pointer(&out)))
	if err := hresult(r); err != nil {
		return nil, err
	}
	return out, nil
}

// await waits for an IAsyncOperation and returns its result in a pointer sized slot.
func await(op object) (uintptr, error) {
	var info object
	r, _, _ := syscall.SyscallN(method(op, slotQueryInterface), uintptr(op),
		uintptr(unsafe.Pointer(&asyncInfoIID)), uintptr(unsafe.Pointer(&info)))
	if err := hresult(r); err != nil {
		return 0, err
	}
	defer release(info)
	for waited := time.Duration(0); ; waited += asyncStep {
		var state int32
		r, _, _ := syscall.SyscallN(method(info, slotAsyncStatus), uintptr(info), uintptr(unsafe.Pointer(&state)))
		if err := hresult(r); err != nil {
			return 0, err
		}
		if state == asyncCompleted {
			break
		}
		if state != asyncStarted {
			return 0, fmt.Errorf("media controls: request ended in state %d", state)
		}
		if waited >= asyncLimit {
			syscall.SyscallN(method(info, slotAsyncCancel), uintptr(info))
			return 0, errors.New("media controls: no answer")
		}
		time.Sleep(asyncStep)
	}
	var result uintptr
	r, _, _ = syscall.SyscallN(method(op, slotGetResults), uintptr(op), uintptr(unsafe.Pointer(&result)))
	return result, hresult(r)
}

func factory() (object, error) {
	class, err := windows.UTF16FromString(managerClass)
	if err != nil {
		return nil, err
	}
	var name uintptr
	r, _, _ := procWindowsCreate.Call(uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)-1), uintptr(unsafe.Pointer(&name)))
	if err := hresult(r); err != nil {
		return nil, err
	}
	defer procWindowsDelete.Call(name)
	var out object
	r, _, _ = procRoGetFactory.Call(name, uintptr(unsafe.Pointer(&managerStaticsIID)), uintptr(unsafe.Pointer(&out)))
	if err := hresult(r); err != nil {
		return nil, err
	}
	return out, nil
}

// currentSession returns the session Windows considers current, nil when there is none.
func currentSession(statics object) (object, error) {
	op, err := getObject(statics, slotRequestAsync)
	if err != nil {
		return nil, err
	}
	defer release(op)
	result, err := await(op)
	if err != nil {
		return nil, err
	}
	manager := *(*object)(unsafe.Pointer(&result))
	if manager == nil {
		return nil, errors.New("media controls: no session manager")
	}
	defer release(manager)
	return getObject(manager, slotGetCurrentSession)
}

func playbackStatus(session object) (int32, error) {
	info, err := getObject(session, slotGetPlaybackInfo)
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, errors.New("media controls: no playback information")
	}
	defer release(info)
	var status int32
	r, _, _ := syscall.SyscallN(method(info, slotPlaybackStatus), uintptr(info), uintptr(unsafe.Pointer(&status)))
	return status, hresult(r)
}

// ask sends a pause or play request and returns whether the session took it.
func ask(session object, slot int) (bool, error) {
	op, err := getObject(session, slot)
	if err != nil {
		return false, err
	}
	defer release(op)
	result, err := await(op)
	return byte(result) != 0, err
}

// worker keeps one thread in the multithreaded apartment for every WinRT call.
type worker struct {
	calls   chan func()
	statics object
	broken  error
}

// system starts the worker the first time media is touched.
var system = sync.OnceValue(func() *worker {
	w := &worker{calls: make(chan func())}
	go func() {
		runtime.LockOSThread()
		r, _, _ := procRoInitialize.Call(1) // RO_INIT_MULTITHREADED
		if w.broken = hresult(r); w.broken == nil {
			w.statics, w.broken = factory()
		}
		for call := range w.calls {
			call()
		}
	}()
	return w
})

func (w *worker) do(call func()) {
	finished := make(chan struct{})
	w.calls <- func() {
		defer close(finished)
		call()
	}
	<-finished
}

// SystemMedia is the desktop's media players: the current session of the system media
// transport controls. Where those cannot be reached, auto-pause does nothing. There is no
// fallback to the play/pause media key: it is a toggle, and after the dongle's own Pause
// (firmware 0.1.65 and newer) it would start the music again with the buds out.
func SystemMedia() Media { return smtcMedia{} }

type smtcMedia struct{}

// PauseActive pauses the current session when it is playing.
func (smtcMedia) PauseActive() (Paused, error) {
	var claim Paused
	var failed error
	w := system()
	w.do(func() {
		if w.broken != nil {
			failed = fmt.Errorf("%w: %v", errNoTransportControls, w.broken)
			return
		}
		claim, failed = pauseCurrent(w.statics)
	})
	return claim, failed
}

func pauseCurrent(statics object) (Paused, error) {
	session, err := currentSession(statics)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	keep := false
	defer func() {
		if !keep {
			release(session)
		}
	}()
	status, err := playbackStatus(session)
	if err != nil || status != statusPlaying {
		return nil, err
	}
	taken, err := ask(session, slotTryPauseAsync)
	if err != nil {
		return nil, err
	}
	if !taken {
		return nil, errNoTransportControls
	}
	for waited := time.Duration(0); waited < pauseSettle; waited += asyncStep {
		if status, err := playbackStatus(session); err == nil && status == statusPaused {
			keep = true
			return &smtcPaused{session: session}, nil
		}
		time.Sleep(asyncStep)
	}
	return nil, errors.New("the player did not confirm the pause")
}

// smtcPaused holds on to the session it paused, so a resume can only reach that player.
type smtcPaused struct{ session object }

// StillOurs implements Paused.
func (p *smtcPaused) StillOurs() bool {
	var ours bool
	system().do(func() {
		if p.session != nil {
			status, err := playbackStatus(p.session)
			ours = err == nil && status == statusPaused
		}
	})
	return ours
}

// Resume implements Paused.
func (p *smtcPaused) Resume() error {
	var failed error
	system().do(func() {
		if p.session == nil {
			return
		}
		status, err := playbackStatus(p.session)
		if err != nil || status != statusPaused {
			failed = err
			return
		}
		_, failed = ask(p.session, slotTryPlayAsync)
	})
	return failed
}

// Release implements Paused.
func (p *smtcPaused) Release() {
	system().do(func() {
		release(p.session)
		p.session = nil
	})
}
