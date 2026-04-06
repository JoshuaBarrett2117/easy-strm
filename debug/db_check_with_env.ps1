# Set environment variables and run database check
$env:PG_HOST = "192.168.31.12"
$env:PG_PORT = "15432"
$env:PG_DATABASE = "easy_strm"
$env:PG_USER = "joshua"
$env:PG_PASSWORD = "dwwzbuwioalvb"

Write-Host "Environment variables set" -ForegroundColor Green

# Load required assemblies
Add-Type -AssemblyName System.Data

Write-Host "`nDatabase Connection Parameters:" -ForegroundColor Yellow
Write-Host "  Host: $env:PG_HOST" -ForegroundColor Gray
Write-Host "  Port: $env:PG_PORT" -ForegroundColor Gray
Write-Host "  Database: $env:PG_DATABASE" -ForegroundColor Gray
Write-Host "  User: $env:PG_USER" -ForegroundColor Gray

# Try to connect using ODBC
try {
    Write-Host "`nAttempting to connect to database..." -ForegroundColor Yellow

    $connStr = "Driver={PostgreSQL Unicode};Server=$env:PG_HOST;Port=$env:PG_PORT;Database=$env:PG_DATABASE;Uid=$env:PG_USER;Pwd=$env:PG_PASSWORD;"
    Write-Host "Connection String: $connStr" -ForegroundColor Gray

    $conn = New-Object System.Data.Odbc.OdbcConnection($connStr)
    $conn.Open()
    Write-Host "Database connection successful!" -ForegroundColor Green

    $cmd = $conn.CreateCommand()
    $cmd.CommandText = "SELECT id, name, password FROM t_user WHERE name = 'admin'"
    $reader = $cmd.ExecuteReader()

    if ($reader.Read()) {
        Write-Host "`nAdmin user found:" -ForegroundColor Green
        Write-Host "  ID: $($reader['id'])" -ForegroundColor Gray
        Write-Host "  Name: $($reader['name'])" -ForegroundColor Gray
        Write-Host "  Password (MD5): $($reader['password'])" -ForegroundColor Gray
    } else {
        Write-Host "`nAdmin user NOT found in database!" -ForegroundColor Red
        Write-Host "Creating admin user..." -ForegroundColor Yellow

        $reader.Close()

        # Create admin user
        $createCmd = $conn.CreateCommand()
        $createCmd.CommandText = "INSERT INTO t_user (name, password) VALUES ('admin', '21232f297a57a5a743894a0e4a801fc3') RETURNING id"
        $newId = $createCmd.ExecuteScalar()
        Write-Host "Admin user created with ID: $newId" -ForegroundColor Green
    }

    if ($reader -and -not $reader.IsClosed) {
        $reader.Close()
    }
    $conn.Close()
} catch {
    Write-Host "`nDatabase connection failed: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Full error: $_" -ForegroundColor Red
}
