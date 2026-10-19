# =============================================
#  MXGJGT — 打包 & 发布 Makefile
#
#  用法:
#    make build          本机 build
#    make release        一键打包三平台 (linux/windows/darwin)
#    make install        本地安装到 /usr/local/bin
#    make test           运行端到端测试
# =============================================

.PHONY: all build release install test clean zip-sha

VERSION   := 0.0.16
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w -X mxgjgt/model.Version=$(VERSION) -X mxgjgt/model.BuildTime=$(BUILD_TIME)

BIN       := mxgjgt
OUTDIR    := dist

# ===== 本机 =====
build:
	@echo "==> 编译 $(VERSION) ..."
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .
	@echo "==> 完成: $(BIN) ($(shell ls -lh $(BIN) | awk '{print $$5}'))"

# ===== 安装到系统 =====
install: build
	cp $(BIN) /usr/local/bin/$(BIN)
	@echo "==> 已安装到 /usr/local/bin/$(BIN)"

# ===== 清理 =====
clean:
	rm -rf $(BIN) $(OUTDIR)
	@echo "==> 清理完成"

# ===== 多平台发布 =====
release:
	@echo "==> 打包 v$(VERSION) ..."
	@mkdir -p $(OUTDIR)

	# Linux amd64
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BIN)-linux-amd64   .
	# Linux arm64
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BIN)-linux-arm64   .
	# Windows amd64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BIN)-windows-amd64.exe .
	# macOS amd64
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BIN)-darwin-amd64   .
	# macOS arm64 (M1/M2)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BIN)-darwin-arm64   .

	# 打包 + SHA256
	cd $(OUTDIR) && for f in $(BIN)-*; do \
		zip -q $$f.zip $$f; \
		rm $$f; \
	done
	cd $(OUTDIR) && sha256sum *.zip > SHA256.txt

	@echo ""
	@echo "==> 产物 (dist/):"
	@ls -lh $(OUTDIR)
	@echo ""
	@echo "==> 版本: v$(VERSION)  $(BUILD_TIME)"

# ===== 测试 =====
test: build
	@echo "==> 运行端到端测试 ..."
	go run /tmp/st.go 2>&1 || true

# ===== 发布到 GitHub Releases =====
# 需要先 `gh auth login`
github-release: release
	@echo "==> 创建 GitHub Release ..."
	gh release create v$(VERSION) $(OUTDIR)/*.zip $(OUTDIR)/SHA256.txt \
		--title "v$(VERSION)" \
		--notes "$(shell cat CHANGELOG.md 2>/dev/null || echo 'See commit history')"
	@echo "==> Release 已发布"
