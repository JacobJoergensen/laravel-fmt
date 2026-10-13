<?php

declare(strict_types=1);

namespace JacobJoergensen\LaravelFmt;

use Composer\CaBundle\CaBundle;
use Composer\InstalledVersions;
use PharData;
use RuntimeException;
use ZipArchive;

final class BinaryDownloader
{
    private const string PACKAGE = 'jacobjoergensen/laravel-fmt';
    private const string REPO = 'JacobJoergensen/laravel-fmt';
    private const string PROJECT = 'laravel-fmt';

    private readonly string $binDir;

    public function __construct()
    {
        $this->binDir = dirname(__DIR__).'/bin';
    }

    public function ensure(): string
    {
        [$os, $arch, $archiveExt, $binaryInArchive] = $this->platform();
        $cached = $this->cachedPath($os, $arch);

        if ($this->isUsable($cached)) {
            return $cached;
        }

        if (! is_dir($this->binDir) && ! mkdir($this->binDir, 0o755, true) && ! is_dir($this->binDir)) {
            throw new RuntimeException(sprintf('Could not create cache directory %s.', $this->binDir));
        }

        $version = $this->prettyVersion();

        $fileVersion = preg_replace('/^v(?=\d)/', '', $version);

        $archiveName = sprintf('%s_%s_%s_%s.%s', self::PROJECT, $fileVersion, $os, $arch, $archiveExt);
        $checksumsName = sprintf('%s_%s_checksums.txt', self::PROJECT, $fileVersion);
        $base = sprintf('https://github.com/%s/releases/download/%s/', self::REPO, $version);

        $archiveBytes = $this->httpGet($base.$archiveName);
        $checksums = $this->httpGet($base.$checksumsName);

        $this->verifyChecksum($archiveName, $archiveBytes, $checksums);

        return $this->extract($archiveBytes, $archiveExt, $binaryInArchive, $cached);
    }

    private function isUsable(string $path): bool
    {
        return is_file($path) && filesize($path) > 0 && is_executable($path);
    }

    /**
     * @return array{0: string, 1: string, 2: string, 3: string}
     */
    private function platform(): array
    {
        $os = match (PHP_OS_FAMILY) {
            'Windows' => 'Windows',
            'Darwin' => 'Darwin',
            'Linux' => 'Linux',
            default => throw new RuntimeException(sprintf('Unsupported operating system: %s.', PHP_OS_FAMILY)),
        };

        $machine = strtolower(php_uname('m'));

        $arch = match ($machine) {
            'x86_64', 'amd64' => 'x86_64',
            'arm64', 'aarch64' => 'arm64',
            default => throw new RuntimeException(sprintf('Unsupported architecture: %s.', $machine)),
        };

        $ext = $os === 'Windows' ? 'zip' : 'tar.gz';
        $binaryInArchive = self::PROJECT.($os === 'Windows' ? '.exe' : '');

        return [$os, $arch, $ext, $binaryInArchive];
    }

    private function cachedPath(string $os, string $arch): string
    {
        $suffix = $os === 'Windows' ? '.exe' : '';

        return sprintf('%s/%s-%s-%s%s', $this->binDir, self::PROJECT, $os, $arch, $suffix);
    }

    private function prettyVersion(): string
    {
        if (class_exists(InstalledVersions::class) && InstalledVersions::isInstalled(self::PACKAGE)) {
            $pretty = InstalledVersions::getPrettyVersion(self::PACKAGE);

            if ($pretty !== null && ! str_contains($pretty, 'dev')) {
                return $pretty;
            }
        }

        throw new RuntimeException(
            'Could not determine the laravel-fmt release to download. Install a tagged version via Composer.',
        );
    }

    private function httpGet(string $url): string
    {
        if (! function_exists('curl_init')) {
            throw new RuntimeException('The PHP cURL extension is required to download the laravel-fmt binary.');
        }

        $handle = curl_init($url);

        // A Windows PHP build ships no CA bundle, so curl cannot verify GitHub
        // unless curl.cainfo is set; Composer's own bundle covers that case.
        $caBundle = CaBundle::getSystemCaRootBundlePath();

        curl_setopt_array($handle, [
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_FOLLOWLOCATION => true, // GitHub release assets 302 to a CDN
            CURLOPT_FAILONERROR => true,
            CURLOPT_USERAGENT => 'laravel-fmt-composer',
            CURLOPT_TIMEOUT => 120,
        ]);

        if ($caBundle !== '') {
            curl_setopt($handle, is_dir($caBundle) ? CURLOPT_CAPATH : CURLOPT_CAINFO, $caBundle);
        }

        $body = curl_exec($handle);
        $error = curl_error($handle);

        if (! is_string($body)) {
            throw new RuntimeException(sprintf('Download failed for %s: %s', $url, $error));
        }

        return $body;
    }

    private function verifyChecksum(string $archiveName, string $archiveBytes, string $checksums): void
    {
        $expected = null;

        foreach (preg_split('/\R/', $checksums) ?: [] as $line) {
            $parts = preg_split('/\s+/', trim($line));

            if (is_array($parts) && count($parts) >= 2 && $parts[1] === $archiveName) {
                $expected = strtolower($parts[0]);

                break;
            }
        }

        if ($expected === null) {
            throw new RuntimeException(sprintf('No checksum listed for %s.', $archiveName));
        }

        if (! hash_equals($expected, hash('sha256', $archiveBytes))) {
            throw new RuntimeException(sprintf('Checksum mismatch for %s.', $archiveName));
        }
    }

    private function extract(string $archiveBytes, string $archiveExt, string $binaryInArchive, string $cached): string
    {
        $tmpDir = $this->binDir.'/.tmp-'.bin2hex(random_bytes(6));

        if (! mkdir($tmpDir, 0o755, true) && ! is_dir($tmpDir)) {
            throw new RuntimeException(sprintf('Could not create temp directory %s.', $tmpDir));
        }

        try {
            $archiveFile = $tmpDir.'/archive.'.$archiveExt;

            // A short write leaves a truncated archive that the checksum above
            // cannot catch, because that ran against the bytes in memory.
            if (file_put_contents($archiveFile, $archiveBytes) !== strlen($archiveBytes)) {
                throw new RuntimeException('Could not write the downloaded archive to disk.');
            }

            if ($archiveExt === 'zip') {
                $this->extractZip($archiveFile, $tmpDir);
            } else {
                $this->extractTar($archiveFile, $tmpDir);
            }

            $extracted = $tmpDir.'/'.$binaryInArchive;

            if (! is_file($extracted)) {
                throw new RuntimeException(sprintf('Archive did not contain %s.', $binaryInArchive));
            }

            // Executable before it is in place, so another process never finds
            // it unusable there and starts a second download.
            if (PHP_OS_FAMILY !== 'Windows' && ! chmod($extracted, 0o755)) {
                throw new RuntimeException(sprintf('Could not make %s executable.', $extracted));
            }

            // Windows will not replace a binary another process is running: a
            // concurrent first run got there first, so its copy is used.
            if (! @rename($extracted, $cached) && ! $this->isUsable($cached)) {
                throw new RuntimeException(sprintf('Could not move the binary to %s.', $cached));
            }

            return $cached;
        } finally {
            $this->removeDir($tmpDir);
        }
    }

    private function extractTar(string $archiveFile, string $dir): void
    {
        $archive = new PharData($archiveFile);

        if (! $archive->extractTo($dir, null, true)) {
            throw new RuntimeException('Could not extract the downloaded archive.');
        }
    }

    private function extractZip(string $archiveFile, string $dir): void
    {
        if (! class_exists(ZipArchive::class)) {
            throw new RuntimeException(
                'The PHP zip extension is required to unpack the laravel-fmt binary on Windows.',
            );
        }

        $zip = new ZipArchive();

        if ($zip->open($archiveFile) !== true) {
            throw new RuntimeException('Could not open the downloaded zip archive.');
        }

        $extracted = $zip->extractTo($dir);
        $zip->close();

        if (! $extracted) {
            throw new RuntimeException('Could not extract the downloaded zip archive.');
        }
    }

    private function removeDir(string $dir): void
    {
        if (! is_dir($dir)) {
            return;
        }

        foreach (scandir($dir) ?: [] as $item) {
            if ($item === '.' || $item === '..') {
                continue;
            }

            $path = $dir.'/'.$item;

            if (is_dir($path)) {
                $this->removeDir($path);
            } else {
                @unlink($path);
            }
        }

        @rmdir($dir);
    }
}
