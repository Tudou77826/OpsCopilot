package com.opscopilot.jdbc;

import java.lang.management.ManagementFactory;
import java.util.*;
import java.util.regex.Pattern;
import javax.management.ObjectName;
import javax.management.openmbean.CompositeData;

/** Strict allowlist: never return arbitrary JVM properties or environment values. */
final class RuntimeDetails {
    private static final Pattern MEMORY_OPTION = Pattern.compile("(?:-Xm[sx]|-XX:(?:InitialHeapSize|MaxHeapSize|MaxDirectMemorySize)=)[0-9]{1,19}[kKmMgGtT]?");

    static Map<String, Object> capture() {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("javaVersion", System.getProperty("java.version", ""));
        result.put("javaRuntimeVersion", System.getProperty("java.runtime.version", ""));
        result.put("javaVendor", System.getProperty("java.vendor", ""));
        result.put("vmName", System.getProperty("java.vm.name", ""));
        result.put("osArch", System.getProperty("os.arch", ""));
        result.put("maxHeapBytes", Runtime.getRuntime().maxMemory());
        List<String> arguments = ManagementFactory.getRuntimeMXBean().getInputArguments();
        List<String> memory = memoryOptions(arguments);
        result.put("memoryOptions", memory);
        result.put("otherOptionCount", Math.max(0, arguments.size() - memory.size()));
        result.put("optionEnvironment", optionEnvironment(System.getenv()));
        long direct = directFromArguments(arguments);
        String source = direct >= 0 ? "startup-argument" : "unavailable";
        // Present in full JDKs/new private images; older jlink images may omit
        // this optional MXBean. No additional module export is required.
        try {
            Object value = ManagementFactory.getPlatformMBeanServer().invoke(
                new ObjectName("com.sun.management:type=HotSpotDiagnostic"), "getVMOption",
                new Object[]{"MaxDirectMemorySize"}, new String[]{"java.lang.String"});
            if (value instanceof CompositeData data) {
                direct = Long.parseLong(String.valueOf(data.get("value")));
                source = direct == 0 ? "vm-default" : "vm-option";
            }
        } catch (Exception | LinkageError ignored) { /* Keep the explicitly labelled argument evidence. */ }
        result.put("directMemoryLimitBytes", direct);
        result.put("directMemorySource", source);
        return result;
    }

    static List<String> memoryOptions(List<String> arguments) {
        return arguments.stream().filter(value -> MEMORY_OPTION.matcher(value).matches()).limit(16).toList();
    }

    static List<String> optionEnvironment(Map<String, String> environment) {
        return List.of("JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS").stream()
            .filter(name -> environment.containsKey(name) && !environment.get(name).isBlank()).toList();
    }

    static long directFromArguments(List<String> arguments) {
        long direct = -1;
        for (String argument : arguments) {
            if (!argument.startsWith("-XX:MaxDirectMemorySize=") || !MEMORY_OPTION.matcher(argument).matches()) continue;
            String size = argument.substring("-XX:MaxDirectMemorySize=".length());
            try {
                char suffix = Character.toLowerCase(size.charAt(size.length() - 1));
                int power = switch (suffix) { case 'k' -> 1; case 'm' -> 2; case 'g' -> 3; case 't' -> 4; default -> 0; };
                long value = Long.parseLong(power > 0 ? size.substring(0, size.length() - 1) : size);
                for (int i = 0; i < power; i++) value = Math.multiplyExact(value, 1024);
                direct = value;
            } catch (NumberFormatException | ArithmeticException ignored) { direct = -1; }
        }
        return direct;
    }
}
