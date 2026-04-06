# Download and use Npgsql to connect to PostgreSQL
$ErrorActionPreference = "Continue"

Write-Host "Downloading Npgsql..." -ForegroundColor Yellow

# Npgsql NuGet package URL
$npgsqlUrl = "https://www.nuget.org/api/v2/package/Npgsql/4.1.10"
$nupkgPath = "$env:TEMP\npgsql.nupkg"
$zipPath = "$env:TEMP\npgsql.zip"
$extractPath = "$env:TEMP\npgsql"

try {
    # Download the package
    Invoke-WebRequest -Uri $npgsqlUrl -OutFile $nupkgPath -UseBasicParsing
    Write-Host "Downloaded Npgsql package" -ForegroundColor Green

    # Rename to .zip
    Copy-Item $nupkgPath $zipPath -Force

    # Extract the package
    if (Test-Path $extractPath) {
        Remove-Item $extractPath -Recurse -Force
    }
    Expand-Archive -Path $zipPath -DestinationPath $extractPath -Force
    Write-Host "Extracted Npgsql package" -ForegroundColor Green

    # Load Npgsql assembly
    $npgsqlDll = Join-Path $extractPath "lib\netstandard2.0\Npgsql.dll"
    if (Test-Path $npgsqlDll) {
        Add-Type -Path $npgsqlDll
        Write-Host "Loaded Npgsql assembly" -ForegroundColor Green

        # Try to connect to the database
        $connStr = "Host=192.168.31.12;Port=15432;Database=easy_strm;Username=joshua;Password=dwwzbuwioalvb"
        Write-Host "`nConnecting to database..." -ForegroundColor Yellow

        $conn = New-Object Npgsql.NpgsqlConnection($connStr)
        $conn.Open()
        Write-Host "Database connection successful!" -ForegroundColor Green

        # Query the admin user
        $cmd = $conn.CreateCommand()
        $cmd.CommandText = "SELECT id, name, password FROM t_user WHERE name = 'admin'"
        $reader = $cmd.ExecuteReader()

        if ($reader.Read()) {
            Write-Host "`nAdmin user found:" -ForegroundColor Green
            Write-Host "  ID: $($reader['id'])" -ForegroundColor Gray
            Write-Host "  Name: $($reader['name'])" -ForegroundColor Gray
            Write-Host "  Password (MD5): $($reader['password'])" -ForegroundColor Gray

            # Check if password matches
            $expectedPassword = "21232f297a57a5a743894a0e4a801fc3"
            if ($reader['password'] -eq $expectedPassword) {
                Write-Host "`nPassword matches expected value!" -ForegroundColor Green
            } else {
                Write-Host "`nPassword does NOT match!" -ForegroundColor Red
                Write-Host "  Expected: $expectedPassword" -ForegroundColor Gray
                Write-Host "  Actual:   $($reader['password'])" -ForegroundColor Gray
            }
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
    } else {
        Write-Host "Npgsql.dll not found at: $npgsqlDll" -ForegroundColor Red
        Write-Host "Available files:" -ForegroundColor Gray
        Get-ChildItem -Path $extractPath -Recurse -Filter "*.dll" | ForEach-Object { Write-Host "  $($_.FullName)" -ForegroundColor Gray }
    }
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Full error: $_" -ForegroundColor Red
}
