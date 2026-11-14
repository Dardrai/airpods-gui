//go:build windows

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

type registryValue struct{}

func (registryValue) Get() (string, bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, RunKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(RunValue)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (registryValue) Set(value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, RunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(RunValue, value)
}

func (registryValue) Delete() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, RunKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(RunValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// New returns the Windows entry for the running executable.
func New() (Manager, error) {
	exe, err := executable()
	if err != nil {
		return nil, err
	}
	return RunEntry{Store: registryValue{}, Exe: exe}, nil
}
