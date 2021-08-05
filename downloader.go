package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	filePerm    = 0666
	dirPerm     = 0755
	downloadDir = "./downloads"
	maxErrors   = 10
	maxDirSize  = 120
)

type manifest struct {
	ProductID int64     `json:"productId"`
	Handle    string    `json:"productHandle"`
	Created   time.Time `json:"created"`
	Images    []image   `json:"images"`
}

func (srv *server) download(ctx context.Context, url string) error {
	name := time.Now().Format("060102_150405")

	reportDir := filepath.Join(downloadDir, name)
	if exists, _ := fileStatus(reportDir); exists {
		return fmt.Errorf("Report directory '%s' already exist", reportDir)
	}
	if err := os.Mkdir(reportDir, dirPerm); err != nil {
		return fmt.Errorf("Report directory '%s' could not be created. cause=%+v", reportDir, err)
	}

	products, err := srv.readReport(url)
	if err != nil {
		return fmt.Errorf("Could not read download link. cause=%+v\n", err)
	}

	dirSize := calcDirSize(len(products))
	useBatches := len(products) > dirSize

	sort.SliceStable(products, func(i, j int) bool {
		return products[i].Handle < products[j].Handle
	})

	countImages := 0
	for _, p := range products {
		countImages += len(p.Images)
	}
	log.Printf("Start downloading; products=%d; images=%d\n", len(products), countImages)

	productTokens := make(chan bool, 4)
	var wg sync.WaitGroup
	var downloadErr error
	var countErrTotal int32
	var countProductsDone int32

	for i := range products {
		if ctx.Err() != nil {
			break
		}
		if atomic.LoadInt32(&countErrTotal) > maxErrors {
			downloadErr = fmt.Errorf("Too many errors")
			break
		}

		productDir := getProductDir(reportDir, i, dirSize, useBatches, products[i].Handle)
		if err := os.MkdirAll(productDir, dirPerm); err != nil {
			downloadErr = fmt.Errorf("Could not create product dir '%s'; cause=%+v\n", productDir, err)
		}

		productTokens <- true
		wg.Add(1)
		go func(p *product, dir string) {
			countErr := srv.downloadProduct(ctx, p, dir)
			atomic.AddInt32(&countErrTotal, countErr)
			currProductsDone := atomic.AddInt32(&countProductsDone, 1)
			if currProductsDone%20 == 0 {
				log.Printf("Downloading...; %d products of %d done\n", currProductsDone, len(products))
			}
			wg.Done()
			<-productTokens
		}(products[i], productDir)
	}

	wg.Wait()

	if downloadErr == nil && ctx.Err() == nil {
		countErr := 0
		countDone := 0
		for _, p := range products {
			for i := range p.Images {
				if p.Images[i].Done {
					countDone++
				} else if p.Images[i].Err != nil {
					countErr++
				}
			}
		}
		log.Printf("Download done; products=%d; images=%d, errors=%d; dir=%s\n", len(products), countDone, countErr, reportDir)
	}
	return downloadErr
}

func (srv *server) downloadProduct(ctx context.Context, p *product, productDir string) int32 {
	var wg sync.WaitGroup
	var countErr int32
	isOK := true

	for i := range p.Images {
		if ctx.Err() != nil || atomic.LoadInt32(&countErr) > maxErrors {
			isOK = false
			break
		}

		srv.imageTokens <- true
		wg.Add(1)
		go func(dir string, img *image) {
			if err := srv.downloadImage(img, dir); err != nil {
				img.Err = err
				atomic.AddInt32(&countErr, 1)
				log.Printf("Failed to download '%s', err=%+v\n", img.Src, err)
			}
			wg.Done()
			<-srv.imageTokens
		}(productDir, &p.Images[i])
	}
	wg.Wait()

	if isOK {
		srv.writeManifest(ctx, p, productDir)
	}
	return countErr
}

func (srv *server) downloadImage(img *image, productDir string) error {
	var err error
	img.Filename, err = srcToFilename(img.Src)
	if err != nil {
		return err
	}

	resp, err := srv.httpClient.Get(img.Src)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	imagePath := path.Join(productDir, img.Filename)
	if err := ioutil.WriteFile(imagePath, buff, filePerm); err != nil {
		return err
	}
	img.Hash = toHash(buff)
	img.Done = true
	return nil
}

func (srv *server) writeManifest(ctx context.Context, p *product, productDir string) {
	manifestImages := make([]image, 0, len(p.Images))
	for _, img := range p.Images {
		if img.Err != nil {
			continue
		}
		manifestImages = append(manifestImages, img)
	}
	prodManifest := manifest{ProductID: p.ID, Handle: p.Handle, Created: time.Now().UTC(), Images: manifestImages}

	json, err := json.MarshalIndent(prodManifest, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal manifest json; err=%+v\n", err)
		return
	}

	manifestPath := filepath.Join(productDir, fmt.Sprintf("imagesync-%d.json", p.ID))
	if err := ioutil.WriteFile(manifestPath, json, filePerm); err != nil {
		log.Printf("Could not write manifest file '%s', err=%+v\n", manifestPath, err)
	}
	return
}

func calcDirSize(countProducts int) int {
	if countProducts <= maxDirSize {
		return maxDirSize
	}
	dirSize := maxInt(30, int(math.Ceil(math.Sqrt(float64(countProducts)))))
	dirs := countProducts / dirSize
	lastSize := countProducts % dirSize
	if lastSize < dirSize/2 {
		dirSize += lastSize/dirs + 1
	}
	return dirSize
}

func getProductDir(reportDir string, productCount int, dirSize int, useBatches bool, handle string) string {
	if !useBatches {
		return filepath.Join(reportDir, handle)
	}
	batchDir := strconv.Itoa(productCount / dirSize)
	return filepath.Join(reportDir, batchDir, handle)
}

func srcToFilename(srcURL string) (string, error) {
	src, err := url.Parse(strings.ReplaceAll(srcURL, `\/`, `/`))
	if err != nil {
		return "", err
	}
	filename := path.Base(src.Path)
	return filename, nil
}

func fileStatus(path string) (bool, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}

func toHash(data []byte) string {
	h := sha256.New()
	h.Write(data)
	hash := fmt.Sprintf("%x", h.Sum(nil))
	return hash
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
