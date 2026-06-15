Add-Type -AssemblyName System.Drawing
$root = Join-Path $PSScriptRoot "..\static\icons"
foreach ($size in 192, 512) {
    $bmp = New-Object System.Drawing.Bitmap $size, $size
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.Clear([System.Drawing.Color]::FromArgb(255, 79, 70, 229))
    $white = [System.Drawing.Brushes]::White
    $gold = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(255, 251, 191, 36))
    $g.FillRectangle($gold, [int]($size * 0.12), [int]($size * 0.2), [int]($size * 0.1), [int]($size * 0.6))
    $g.FillRectangle($white, [int]($size * 0.22), [int]($size * 0.18), [int]($size * 0.58), [int]($size * 0.64))
    $path = Join-Path $root "icon-$size.png"
    $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
    $g.Dispose()
    $bmp.Dispose()
}
Write-Output "icons ok"
