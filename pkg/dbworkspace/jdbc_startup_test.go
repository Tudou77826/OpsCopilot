package dbworkspace

import (
	"strings"
	"testing"
)

func TestJavaStartupDiagnosticClassifiesWithoutEchoingEnvironment(t *testing.T) {
	var diagnostic jdbcStartupDiagnostic
	for _, chunk := range []string{"Picked up JAVA_TOOL_OPTIONS: -Dpassword=private-jvm-password\nUnrecognized VM", " option 'PrivateFlag=private-jvm-token'\nCould not create the Java Virtual Machine"} {
		n, err := diagnostic.Write([]byte(chunk))
		if n != len(chunk) || err != nil {
			t.Fatal(n, err)
		}
	}
	if !strings.Contains(diagnostic.summary(), "Java 拒绝启动参数") || strings.Contains(diagnostic.summary(), "private-jvm") || strings.Contains(diagnostic.summary(), "PrivateFlag") {
		t.Fatal("startup detail leaked or classification lost")
	}
	if len(diagnostic.tail) > 256 {
		t.Fatal("unbounded stderr tail")
	}
}
