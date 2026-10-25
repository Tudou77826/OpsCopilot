package com.opscopilot.jdbc;

import org.junit.Test;
import static org.junit.Assert.*;
import java.util.*;

public class RuntimeDetailsTest {
    @Test public void exposesOnlyNumericMemoryOptionsAndEnvironmentNames() {
        List<String> input = List.of("-Xms16m", "-Xmx96m", "-XX:MaxDirectMemorySize=96m", "-XX:MaxDirectMemorySize=192m",
            "-Dpassword=private-jvm-password", "-Dtoken=private-jvm-token", "-javaagent:private-agent", "-Xmx96m password=hidden");
        String selected = RuntimeDetails.memoryOptions(input).toString();
        assertEquals(4, RuntimeDetails.memoryOptions(input).size());
        assertFalse(selected.contains("private")); assertFalse(selected.contains("password"));
        assertEquals(192L << 20, RuntimeDetails.directFromArguments(input));
        assertEquals(List.of("JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS"), RuntimeDetails.optionEnvironment(Map.of(
            "JAVA_TOOL_OPTIONS", "-Dpassword=private", "_JAVA_OPTIONS", "-Xmx160m", "JDK_JAVA_OPTIONS", " ", "OTHER_TOKEN", "hidden")));
    }
    @Test public void handlesAutomaticAndUnavailableDirectMemoryWithoutGuessing() {
        assertEquals(-1, RuntimeDetails.directFromArguments(List.of("-Xmx96m")));
        assertEquals(0, RuntimeDetails.directFromArguments(List.of("-XX:MaxDirectMemorySize=0")));
        assertEquals(-1, RuntimeDetails.directFromArguments(List.of("-XX:MaxDirectMemorySize=9999999999999999999T")));
    }
    @Test public void capturesActualHeapAndVersion() {
        Map<String, Object> info = RuntimeDetails.capture();
        assertEquals(Runtime.getRuntime().maxMemory(), info.get("maxHeapBytes"));
        assertEquals(System.getProperty("java.version"), info.get("javaVersion"));
        assertTrue(info.containsKey("directMemorySource"));
        assertFalse(info.containsKey("environment")); assertFalse(info.containsKey("inputArguments"));
    }
}
