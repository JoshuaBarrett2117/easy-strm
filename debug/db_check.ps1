# Database Connection Test
$ErrorActionPreference = "Continue"

# Load required assemblies
Add-Type -AssemblyName System.Data

# Connection string (using environment variables for security)
$pgHost = $env:PG_HOST
$pgPort = $env:PG_PORT
$pgDatabase = $env:PG_DATABASE
$pgUser = $env:PG_USER
$pgPassword = $env:PG_PASSWORD

Write-Host "Database Connection Parameters:" -ForegroundColor Yellow
Write-Host "  Host: $pgHost" -ForegroundColor Gray
Write-Host "  Port: $pgPort" -ForegroundColor Gray
Write-Host "  Database: $pgDatabase" -ForegroundColor Gray
Write-Host "  User: $pgUser" -ForegroundColor Gray
Write-Host "  Password: ****" -ForegroundColor Gray

if (-not $pgHost -or -not $pgPort -or -not $pgDatabase -or -not $pgUser -or -not $pgPassword) {
    Write-Host "Environment variables not set. Please set PG_HOST, PG_PORT, PG_DATABASE, PG_USER, PG_PASSWORD" -ForegroundColor Red
    exit 1
}

# Try to connect using Npgsql (if available)
try {
    # Try to load Npgsql
    $npgsqlAssembly = [System.Reflection.Assembly]::LoadWithPartialName("Npgsql")
    if ($npgsqlAssembly) {
        Write-Host "`nUsing Npgsql to connect..." -ForegroundColor Yellow

        $connStr = "Host=$pgHost;Port=$pgPort;Database=$pgDatabase;Username=$pgUser;Password=$pgPassword"
        $conn = New-Object Npgsql.NpgsqlConnection($connStr)
        $conn.Open()

        $cmd = $conn.CreateCommand()
        $cmd.CommandText = "SELECT id, name, password FROM t_user WHERE name = 'admin'"
        $reader = $cmd.ExecuteReader()

        if ($reader.Read()) {
            Write-Host "Admin user found:" -ForegroundColor Green
            Write-Host "  ID: $($reader['id'])" -ForegroundColor Gray
            Write-Host "  Name: $($reader['name'])" -ForegroundColor Gray
            Write-Host "  Password (MD5): $($reader['password'])" -ForegroundColor Gray
        } else {
            Write-Host "Admin user NOT found in database!" -ForegroundColor Red
        }

        $reader.Close()
        $conn.Close()
    } else {
        Write-Host "Npgsql not available. Trying ODBC..." -ForegroundColor Yellow

        # Try ODBC connection
        $connStr = "Driver={PostgreSQL Unicode};Server=$pgHost;Port=$pgPort;Database=$pgDatabase;Uid=$pgUser;Pwd=$pgPassword;"
        $conn = New-Object System.Data.Odbc.OdbcConnection($connStr)
        $conn.Open()

        $cmd = $conn.CreateCommand()
        $cmd.CommandText = "SELECT id, name, password FROM t_user WHERE name = 'admin'"
        $reader = $cmd.ExecuteReader()

        if ($reader.Read()) {
            Write-Host "Admin user found:" -ForegroundColor Green
            Write-Host "  ID: $($reader['id'])" -ForegroundColor Gray
            Write-Host "  Name: $($reader['name'])" -ForegroundColor Gray
            Write-Host "  Password (MD5): $($reader['password'])" -ForegroundColor Gray
        } else {
            Write-Host "Admin user NOT found in database!" -ForegroundColor Red
        }

        $reader.Close()
        $conn.Close()
    }
} catch {
    Write-Host "Database connection failed: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Full error: $_" -ForegroundColor Red
}
