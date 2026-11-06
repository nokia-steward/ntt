package k3log_test

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/nokia/ntt/internal/k3log"
)

const text = `20210506T092317.322187|mpar|k3r=(unknown):0|TestcaseExecutor.tests|{}
20210506T092317.322215|mpar|k3r=(unknown):0|TestcaseExecutor.time_out|10000.0
20210506T092317.322225|mpar|k3r=(unknown):0|test.guardTimer|1.0
20210506T092317.322228|tcst|k3r=(unknown):0|test.Fail_A|444.0
20210506T092317.322245|cocr|k3r=(unknown):0|MTC|once
20210506T092317.322335|cost|MTC=test.ttcn3:37|test.Fail_A
20210506T092317.322343|tcen|MTC|test.Fail_A()
20210506T092317.326311|fnen|MTC=test.ttcn3:37|test.waitAndSetVerdict(seconds=1.0,v=fail)
20210506T092317.326331|wait|MTC=test.ttcn3:49|20210506T092318.326323
20210506T092318.326492|bctr|MTC=test.ttcn3:50|test.ttcn3:37:waitAndSetVerdict(seconds=1.0,v=fail)|(unknown):0:Fail_A()
20210506T092318.326525|setv|MTC=test.ttcn3:50|none|fail
20210506T092318.326539|fnlv|MTC=test.ttcn3:37|test.waitAndSetVerdict(seconds=-,v=-)
20210506T092318.326560|tclv|MTC|test.Fail_A()
20210506T092318.326598|cofi|MTC|fail
20210506T092318.326814|tcfi|k3r=(unknown):0|test.Fail_A|fail
`

func Example() {
	r := bufio.NewReader(strings.NewReader(text))

	for {
		text, err := r.ReadString('\n')
		if err != nil {
			break
		}

		e, err := k3log.ParseEvent(text)
		if err != nil {
			fmt.Println(err.Error())
			continue
		}

		switch e.ID() {
		case "tcfi":
			fmt.Printf("Test %s finished with verdict %q", e.Field(3), e.Field(4))
			// Output: Test test.Fail_A finished with verdict "fail"
		}
	}
}

func ExampleEvent_String() {
	e, err := k3log.ParseEvent("20210506T092317.326814|tcfi|k3r=(unknown):0|test.Fail_A|fail\n")
	if err != nil {
		return
	}
	fmt.Println(e)
	// Output: 20210506T092317.326814|tcfi|k3r=(unknown):0|test.Fail_A|fail
}
