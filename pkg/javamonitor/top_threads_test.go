package javamonitor

import "testing"

func TestCountThreadStatesFromJstack(t *testing.T) {
	input := `
"t1" #1 prio=5 os_prio=0 tid=0x00007 nid=0x1a runnable
   java.lang.Thread.State: RUNNABLE
"t2" #2 prio=5 os_prio=0 tid=0x00008 nid=0x1b waiting
   java.lang.Thread.State: WAITING
"t3" #3 prio=5 os_prio=0 tid=0x00009 nid=0x1c timed_waiting
   java.lang.Thread.State: TIMED_WAITING
"t4" #4 prio=5 os_prio=0 tid=0x0000a nid=0x1d blocked
   java.lang.Thread.State: BLOCKED
`
	c := countThreadStatesFromJstack(input)
	if c.Runnable != 1 || c.Waiting != 1 || c.TimedWaiting != 1 || c.Blocked != 1 {
		t.Fatalf("unexpected counts: %+v", c)
	}
}

func TestFindThreadInJstack(t *testing.T) {
	input := `
"http-nio-8080-exec-1" #33 prio=5 os_prio=0 tid=0x00007 nid=0x1a runnable
   java.lang.Thread.State: RUNNABLE
        at com.foo.Bar.baz(Bar.java:1)
`
	name, state, top := findThreadInJstack(input, "0x1a")
	if name != "http-nio-8080-exec-1" {
		t.Fatalf("name=%q", name)
	}
	if state != "RUNNABLE" {
		t.Fatalf("state=%q", state)
	}
	if top == "" {
		t.Fatalf("expected top frame")
	}
}

func TestParseTopHOutput(t *testing.T) {
	topOut := `
top - 10:00:00 up 1 day,  1 user,  load average: 0.00, 0.00, 0.00
Tasks: 100 total,   1 running,  99 sleeping,   0 stopped,   0 zombie
%Cpu(s):  1.0 us,  0.5 sy,  0.0 ni, 98.0 id,  0.0 wa,  0.0 hi,  0.5 si,  0.0 st
PID USER      PR  NI    VIRT    RES    SHR S %CPU %MEM     TIME+ COMMAND
1234 root      20   0  123456  12345   123 S  5.0  0.1   0:00.01 java
2234 root      20   0  123456  12345   123 R 10.0  0.1   0:00.02 java
3234 root      20   0  123456  12345   123 R  3.0  0.1   0:00.03 java
`
	threads := parseTopHOutput(topOut, 1234)
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	if threads[0].tid != 2234 {
		t.Fatalf("expected top tid 2234, got %d", threads[0].tid)
	}
}
