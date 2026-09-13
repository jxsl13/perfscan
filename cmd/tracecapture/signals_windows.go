package main

import "os"

func captureSignals() []os.Signal { return []os.Signal{os.Interrupt} }
