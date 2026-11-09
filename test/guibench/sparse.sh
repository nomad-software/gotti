#!/bin/bash
# A mostly empty screen: a short line every 5 ms for 3 s.
clear
for i in $(seq 1 600); do echo "line $i"; sleep 0.005; done
