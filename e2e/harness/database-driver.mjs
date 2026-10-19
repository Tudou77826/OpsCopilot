// Only imports a driver explicitly supplied by the test caller. No downloads.
export async function ensureMySQLJDBC(page) {
  const jar = process.env.OPSCOPILOT_TEST_JDBC_JAR;
  return page.evaluate(async jar => {
    const api = window.go.main.App;
    const status = await api.DatabaseJDBCStatus();
    if (!status.available) throw new Error('JDBC runtime is required for database UI tests');
    const existing = status.drivers.find(d => d.dialect === 'mysql');
    if (existing) return existing.id;
    if (!jar) throw new Error('Set OPSCOPILOT_TEST_JDBC_JAR to an official local Connector/J JAR');
    const driver = await api.DatabaseImportJDBC({ name: 'MySQL Connector/J', className: 'com.mysql.cj.jdbc.Driver', urlTemplate: 'jdbc:mysql://{host}:{port}/{database}', dialect: 'mysql', paths: [jar] });
    return driver.id;
  }, jar);
}
