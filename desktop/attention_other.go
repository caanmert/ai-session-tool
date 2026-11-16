//go:build !darwin

package main

func setDockAttention(int, bool) {}

func postNotification(string, string) {}
