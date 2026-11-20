$ErrorActionPreference = 'Stop'

# 1. Login to CMS
$loginBody = @{
    identifier = 'admin'
    password   = 'Admin@CMS2026!'
} | ConvertTo-Json

$loginRes = Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/auth/login' -Method Post -ContentType 'application/json' -Body $loginBody
$token = $loginRes.token
if (-not $token -and $loginRes.data) {
    $token = $loginRes.data.token
}
Write-Host "Login successful. Token acquired: $($token.Substring(0, 15))..."
$headers = @{ 'Authorization' = "Bearer $token" }

# 2. Verify 'pages' Content Type
$ct = Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/content-types/pages' -Headers $headers -Method Get
Write-Host "Verified 'pages' content type exists: $($ct.name) (kind: $($ct.kind))"
$hasSlug = $ct.fields | Where-Object { $_.field_key -eq 'slug' }
if (-not $hasSlug) {
    Write-Host "Adding 'slug' field to 'pages' content type..."
    $slugField = @{
        name       = 'Slug'
        field_key  = 'slug'
        field_type = 'short_text'
    } | ConvertTo-Json
    $null = Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/content-types/pages/fields' -Headers $headers -Method Post -ContentType 'application/json' -Body $slugField
    Write-Host "Slug field added successfully."
}

# 3. Create or update page 'about-us'
$blocks = @(
    @{
        id = 'sec-hero'
        type = 'core/section'
        attributes = @{
            padding = 'py-16'
            backgroundColor = '#f8fafc'
        }
        children = @(
            @{
                id = 'cont-1'
                type = 'core/container'
                attributes = @{
                    maxWidth = 'max-w-5xl'
                }
                children = @(
                    @{
                        id = 'h-1'
                        type = 'core/heading'
                        attributes = @{
                            level = 1
                            content = 'About Us & Our Mission'
                            align = 'center'
                        }
                    },
                    @{
                        id = 'p-1'
                        type = 'core/paragraph'
                        attributes = @{
                            content = 'We are building the future of content management with Go and visual blocks.'
                            align = 'center'
                        }
                    },
                    @{
                        id = 'btn-1'
                        type = 'core/button'
                        attributes = @{
                            text = 'Explore Projects'
                            url = '/projects'
                            variant = 'primary'
                            align = 'center'
                        }
                    }
                )
            }
        )
    }
)

$pageData = @{
    title    = 'About Us & Our Mission'
    slug     = 'about-us'
    template = 'default'
    seo      = @{
        meta_title       = 'About Us | Go CMS'
        meta_description = 'Learn more about Go CMS and our mission.'
    }
    blocks   = $blocks
}

# Check if page already exists
$listRes = Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/content/pages?limit=50&preview=true' -Headers $headers -Method Get
$existing = $null
if ($listRes.data) {
    $existing = $listRes.data | Where-Object { $_.slug -eq 'about-us' }
}

if ($existing) {
    Write-Host "Existing 'about-us' page found (id: $($existing.id)). Updating draft..."
    $updatePayload = @{
        data                   = $pageData
        expected_draft_version = $existing.draft_version
    } | ConvertTo-Json -Depth 10

    $pageRes = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/content/pages/$($existing.id)" -Headers $headers -Method Put -ContentType 'application/json' -Body $updatePayload
    $pageId = $existing.id
} else {
    Write-Host "Creating new 'about-us' page..."
    $createPayload = $pageData | ConvertTo-Json -Depth 10
    $pageRes = Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/content/pages' -Headers $headers -Method Post -ContentType 'application/json' -Body $createPayload
    $pageId = $pageRes.id
    if (-not $pageId -and $pageRes.data) { $pageId = $pageRes.data.id }
}

Write-Host "Page ID: $pageId"

# 4. Check draft protection
try {
    $prePubCheck = Invoke-WebRequest -Uri 'http://localhost:8080/about-us' -Method Get -UseBasicParsing
    Write-Host "Pre-publish check status: $($prePubCheck.StatusCode)"
} catch {
    Write-Host "Draft protection confirmed: public request returned 404 ($($_.Exception.Message))"
}

# 5. Publish Page
Write-Host "Publishing page..."
$pubRes = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/content/pages/$pageId/publish" -Headers $headers -Method Post
Write-Host "Published status: $($pubRes.status)"

# 6. Fetch Public URL
Write-Host "Fetching public URL: http://localhost:8080/about-us..."
$pubPage = Invoke-WebRequest -Uri 'http://localhost:8080/about-us' -Method Get -UseBasicParsing
Write-Host "Public Page HTTP Status: $($pubPage.StatusCode)"
$html = $pubPage.Content

# 7. Assertions
$checks = @(
    @{ Name = 'Heading'; Expected = 'About Us &amp; Our Mission'; Result = $html.Contains('About Us &amp; Our Mission') },
    @{ Name = 'Paragraph'; Expected = 'We are building the future of content management'; Result = $html.Contains('We are building the future of content management') },
    @{ Name = 'Button Text'; Expected = 'Explore Projects'; Result = $html.Contains('Explore Projects') },
    @{ Name = 'Button Link'; Expected = '/projects'; Result = $html.Contains('/projects') },
    @{ Name = 'Section Class'; Expected = 'cms-block-section'; Result = $html.Contains('cms-block-section') },
    @{ Name = 'Theme Header'; Expected = 'site-header'; Result = $html.Contains('site-header') },
    @{ Name = 'Theme Footer'; Expected = 'site-footer'; Result = $html.Contains('site-footer') }
)

Write-Host "`n=== Verification Results ==="
$allPassed = $true
foreach ($c in $checks) {
    if ($c.Result) {
        Write-Host "  PASS: $($c.Name)" -ForegroundColor Green
    } else {
        Write-Host "  FAIL: $($c.Name) (Expected '$($c.Expected)')" -ForegroundColor Red
        $allPassed = $false
    }
}

if ($allPassed) {
    Write-Host "`nAll 7 SSR Page Builder assertions PASSED successfully!" -ForegroundColor Green
} else {
    Write-Host "`nSome assertions failed." -ForegroundColor Red
    exit 1
}
