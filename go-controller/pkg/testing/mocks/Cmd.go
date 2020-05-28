// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	io "io"

	mock "github.com/stretchr/testify/mock"
)

// Cmd is an autogenerated mock type for the Cmd type
type Cmd struct {
	mock.Mock
}

// CombinedOutput provides a mock function with given fields:
func (_m *Cmd) CombinedOutput() ([]byte, error) {
	ret := _m.Called()

	var r0 []byte
	if rf, ok := ret.Get(0).(func() []byte); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Output provides a mock function with given fields:
func (_m *Cmd) Output() ([]byte, error) {
	ret := _m.Called()

	var r0 []byte
	if rf, ok := ret.Get(0).(func() []byte); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Run provides a mock function with given fields:
func (_m *Cmd) Run() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDir provides a mock function with given fields: dir
func (_m *Cmd) SetDir(dir string) {
	_m.Called(dir)
}

// SetEnv provides a mock function with given fields: env
func (_m *Cmd) SetEnv(env []string) {
	_m.Called(env)
}

// SetStderr provides a mock function with given fields: out
func (_m *Cmd) SetStderr(out io.Writer) {
	_m.Called(out)
}

// SetStdin provides a mock function with given fields: in
func (_m *Cmd) SetStdin(in io.Reader) {
	_m.Called(in)
}

// SetStdout provides a mock function with given fields: out
func (_m *Cmd) SetStdout(out io.Writer) {
	_m.Called(out)
}

// Start provides a mock function with given fields:
func (_m *Cmd) Start() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// StderrPipe provides a mock function with given fields:
func (_m *Cmd) StderrPipe() (io.ReadCloser, error) {
	ret := _m.Called()

	var r0 io.ReadCloser
	if rf, ok := ret.Get(0).(func() io.ReadCloser); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.ReadCloser)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// StdoutPipe provides a mock function with given fields:
func (_m *Cmd) StdoutPipe() (io.ReadCloser, error) {
	ret := _m.Called()

	var r0 io.ReadCloser
	if rf, ok := ret.Get(0).(func() io.ReadCloser); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.ReadCloser)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Stop provides a mock function with given fields:
func (_m *Cmd) Stop() {
	_m.Called()
}

// Wait provides a mock function with given fields:
func (_m *Cmd) Wait() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
