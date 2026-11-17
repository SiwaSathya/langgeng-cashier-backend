package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StorageService interface {
	UploadFile(ctx context.Context, folder string, filename string, reader io.Reader, size int64, contentType string) (string, error)
}

type MinIOStorageService struct {
	client           *minio.Client
	bucket           string
	publicURL        string
	uploadDir        string
	useLocalFallback bool
}

func NewStorageService() StorageService {
	endpoint := strings.TrimSpace(os.Getenv("MINIO_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("MINIO_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("MINIO_SECRET_KEY"))
	bucket := strings.TrimSpace(os.Getenv("MINIO_BUCKET"))
	if bucket == "" {
		bucket = "stock-opname"
	}
	useSSL := strings.ToLower(strings.TrimSpace(os.Getenv("MINIO_USE_SSL"))) == "true"
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MINIO_PUBLIC_URL")), "/")
	uploadDir := strings.TrimSpace(os.Getenv("UPLOAD_DIR"))
	if uploadDir == "" {
		uploadDir = "./uploads"
	}

	storage := &MinIOStorageService{
		bucket:           bucket,
		publicURL:        publicURL,
		uploadDir:        uploadDir,
		useLocalFallback: true,
	}

	if endpoint != "" && accessKey != "" && secretKey != "" {
		minioClient, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: useSSL,
		})
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			exists, errBucket := minioClient.BucketExists(ctx, bucket)
			if errBucket == nil {
				if !exists {
					_ = minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
				}
				storage.client = minioClient
				storage.useLocalFallback = false
				log.Printf("[STORAGE] MinIO Storage aktif terhubung ke '%s', bucket '%s'", endpoint, bucket)
				return storage
			} else {
				log.Printf("[STORAGE] MinIO belum siap (%v), menggunakan fallback penyimpanan lokal (%s)", errBucket, uploadDir)
			}
		} else {
			log.Printf("[STORAGE] Gagal inisialisasi MinIO client (%v), menggunakan fallback penyimpanan lokal (%s)", err, uploadDir)
		}
	} else {
		log.Printf("[STORAGE] Kredensial MinIO belum diset lengkap di .env, menggunakan fallback penyimpanan lokal (%s)", uploadDir)
	}

	return storage
}

func (s *MinIOStorageService) UploadFile(ctx context.Context, folder string, filename string, reader io.Reader, size int64, contentType string) (string, error) {
	if folder == "" {
		folder = "opname"
	}
	ext := filepath.Ext(filename)
	cleanBase := strings.TrimSuffix(filepath.Base(filename), ext)
	cleanBase = strings.ReplaceAll(cleanBase, " ", "_")
	if len(cleanBase) > 30 {
		cleanBase = cleanBase[:30]
	}

	now := time.Now()
	uniqueID := uuid.New().String()[:8]
	objectName := fmt.Sprintf("%s/%04d/%02d/%s_%s%s", folder, now.Year(), int(now.Month()), cleanBase, uniqueID, ext)

	// Jika MinIO aktif dan berhasil terkoneksi
	if !s.useLocalFallback && s.client != nil {
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		_, err := s.client.PutObject(ctx, s.bucket, objectName, reader, size, minio.PutObjectOptions{
			ContentType: contentType,
		})
		if err == nil {
			if s.publicURL != "" {
				return fmt.Sprintf("%s/%s/%s", s.publicURL, s.bucket, objectName), nil
			}
			endpoint := strings.TrimSpace(os.Getenv("MINIO_ENDPOINT"))
			scheme := "http"
			if strings.ToLower(strings.TrimSpace(os.Getenv("MINIO_USE_SSL"))) == "true" {
				scheme = "https"
			}
			return fmt.Sprintf("%s://%s/%s/%s", scheme, endpoint, s.bucket, objectName), nil
		}
		log.Printf("[STORAGE] Gagal upload ke MinIO (%v), beralih ke penyimpanan lokal", err)
	}

	// Fallback penyimpanan lokal
	localDestPath := filepath.Join(s.uploadDir, filepath.FromSlash(objectName))
	if err := os.MkdirAll(filepath.Dir(localDestPath), 0755); err != nil {
		return "", fmt.Errorf("gagal membuat folder upload lokal: %w", err)
	}

	dstFile, err := os.Create(localDestPath)
	if err != nil {
		return "", fmt.Errorf("gagal membuat file lokal: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, reader); err != nil {
		return "", fmt.Errorf("gagal menyimpan file ke lokal: %w", err)
	}

	return "/" + filepath.ToSlash(localDestPath), nil
}
