#!/bin/sh
# Updates the widget's translations: extracts the strings from the QML
# files into template.pot, merges them into every <lang>.po and compiles
# the .po files into the package (contents/locale/<lang>/LC_MESSAGES).
# Needs gettext (xgettext, msgmerge, msgfmt).
#
#   plasmoid/translate/update.sh          extract, merge, compile
#   plasmoid/translate/update.sh compile  only compile (CI, packaging)
set -eu
cd "$(dirname "$0")"
domain=plasma_applet_org.doormutex.trafficlight

if [ "${1:-}" != compile ]; then
	xgettext --from-code=UTF-8 -C -kde -ci18n -ki18n:1 -ki18nc:1c,2 -ki18np:1,2 -ki18ncp:1c,2,3 \
		--package-name=doormutex-plasmoid --msgid-bugs-address=https://github.com/padalton/doormutex/issues \
		--sort-by-file --add-location=file -o template.pot ../package/contents/ui/*.qml
	# Stable between runs, and a header with our licence instead of xgettext's placeholder.
	sed -i -e '/^"POT-Creation-Date:/d' -e '1,4c\
# Strings of the doormutex Plasma widget – copy to <lang>.po to translate.\
# SPDX-FileCopyrightText: 2026 The doormutex contributors\
# SPDX-License-Identifier: Apache-2.0' template.pot
	for po in *.po; do
		[ -e "$po" ] || continue
		msgmerge --quiet --update --backup=none --no-fuzzy-matching --add-location=file "$po" template.pot
	done
fi

for po in *.po; do
	[ -e "$po" ] || continue
	lang=${po%.po}
	mkdir -p "../package/contents/locale/$lang/LC_MESSAGES"
	msgfmt --check -o "../package/contents/locale/$lang/LC_MESSAGES/$domain.mo" "$po"
done
