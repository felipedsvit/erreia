BUNDLE := $(shell find $(HOME)/.local/share/gem -name bundle -type f 2>/dev/null | head -1)
ifeq ($(BUNDLE),)
BUNDLE := bundle
endif

.PHONY: serve build clean install

install:
	$(BUNDLE) install

serve:
	$(BUNDLE) exec jekyll serve --config _config.yml,_config.dev.yml --livereload

build:
	$(BUNDLE) exec jekyll build

clean:
	$(BUNDLE) exec jekyll clean
	rm -rf _site
