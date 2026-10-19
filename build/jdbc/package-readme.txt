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

Connection diagnostics v3: use Test connection or Connect normally. Each attempt
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
