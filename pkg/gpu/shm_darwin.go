package gpu

/*
#include <stdlib.h>
#include <sys/mman.h>
#include <fcntl.h>
#include <unistd.h>
#include <string.h>

// shmCreate creates a POSIX shared memory segment, sizes it, and maps it.
// Returns the mapped pointer and fd, or NULL on error.
static void* shmCreate(const char* name, int size, int* fdOut) {
	int fd = shm_open(name, O_CREAT|O_RDWR, 0600);
	if (fd < 0) return NULL;
	if (ftruncate(fd, size) < 0) {
		close(fd);
		shm_unlink(name);
		return NULL;
	}
	void* ptr = mmap(NULL, size, PROT_READ|PROT_WRITE, MAP_SHARED, fd, 0);
	if (ptr == MAP_FAILED) {
		close(fd);
		shm_unlink(name);
		return NULL;
	}
	*fdOut = fd;
	return ptr;
}

static void shmDestroy(const char* name, void* ptr, int size, int fd) {
	if (ptr != NULL && ptr != MAP_FAILED) munmap(ptr, size);
	if (fd >= 0) close(fd);
	shm_unlink(name);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// shmSegment holds a POSIX shared memory segment.
type shmSegment struct {
	name string
	ptr  unsafe.Pointer
	size int
	fd   C.int
}

// shmNew creates a named POSIX shared memory segment of the given size.
func shmNew(name string, size int) (*shmSegment, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var fd C.int
	ptr := C.shmCreate(cname, C.int(size), &fd)
	if ptr == nil {
		return nil, fmt.Errorf("shm_open %s failed", name)
	}
	return &shmSegment{name: name, ptr: ptr, size: size, fd: fd}, nil
}

// Bytes returns the mapped memory as a Go byte slice.
func (s *shmSegment) Bytes() []byte {
	return unsafe.Slice((*byte)(s.ptr), s.size)
}

// Destroy unmaps and unlinks the segment.
func (s *shmSegment) Destroy() {
	cname := C.CString(s.name)
	defer C.free(unsafe.Pointer(cname))
	C.shmDestroy(cname, s.ptr, C.int(s.size), s.fd)
	s.ptr = nil
}
