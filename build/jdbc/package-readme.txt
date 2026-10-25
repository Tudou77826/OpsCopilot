OpsCopilot - JDBC edition

Extract the entire archive and run opscopilot.exe. Keep jdbc-runtime beside
the executable. No system Java/JDK installation or JAVA_HOME setting is needed.

In Database > Add connection > Manage drivers, import the trusted vendor JDBC
JAR and any dependency JARs, then select that driver in the connection form.
Vendor database drivers are not bundled. All database connections use imported JDBC drivers.

This archive contains application defaults and the private Java runtime.
Personal settings, saved database connections, credentials and test data are
not included. Configure a connection before testing.

Java and third-party license notices are retained under jdbc-runtime.

Connection diagnostics v4: use Test connection or Connect normally. Each attempt
that passes profile/credential validation writes a separate report under
database-diagnostics beside the application configuration (normally beside the
executable). The most recent 20 reports are retained, including successful tests.
On failure, Copy error details also copies the report and its saved file path.
If that directory is not writable, copy the report from the error panel instead.

Reports contain actual SSH routes/accounts, channel outcomes, filtered JDBC
settings and stage timings. Passwords, SQL, result rows and wire payloads are
not recorded. Internal addresses and SSH account names remain in these reports.
No extra network probes, remote commands or diagnostic SQL are performed.
When comparing configurations, change one field per test and retain both logs.

Both successful and failed reports use the same comparison fields: actual Java
build and heap limit, numeric memory arguments received by the JVM, direct-memory
limit with its evidence source, component and driver SHA256 fingerprints, and
the configured database target, effective URL and SSH route. Missing fields are
explicitly unknown. Automatic memory limits are not guessed.
Only the presence of JVM option environment variables is recorded, never their
values or arbitrary JVM arguments. Startup stderr is classified without copying
its raw text. Driver stack samples contain only classes, methods and line numbers.

For a JDBC direct-buffer allocation failure, jdbc-memory-check.cmd offers a
96 MiB baseline and a 1024 MiB comparison. Exit OpsCopilot first, choose one mode,
then test the same saved connection once. Keep both diagnostic reports. The
override applies only to that launch; it does not change Java heap size, server
settings, saved profiles, or retry operations. Allocation failure alone does not
prove that increasing memory is correct: compare the working client's exact
driver JAR, JDBC URL, target port and driver buffer settings as well.
Normal launches preserve the JVM's automatic direct-memory policy. Collect one
normal-launch report in each environment first; the comparison menu explicitly
sets a limit and can change the behavior you are investigating.

SSH connections: select the saved final SSH node, including its jump-host
configuration. Fill the database endpoint as reachable from that final node.
For a database listening only on that node's loopback, use 127.0.0.1.

The program handles transport selection internally. Standard forwarding is tried
first; only an explicit SSH policy rejection enables the temporary Python 3 relay
on the same node, account and database target. No mode selection is required.
The relay requires python3 and SSH command execution, uploads no files, creates
no remote listening service and requires no remote Java installation. Database
authentication failures and SQL are not automatically retried. Internal routes
and transport outcomes remain available in detailed diagnostic reports.

Gauss 100 preset: com.huawei.gauss.jdbc.ZenithDriver
URL template: jdbc:zenith:@{host}:{port}
