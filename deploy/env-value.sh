#!/bin/sh
# Prints one setting from a .env file, as Docker Compose reads it:
#   sh deploy/env-value.sh HP_DOMAIN deploy/.env
# Takes the last line for the name; allows "export ", spaces around "=",
# single or double quotes, a comment after an unquoted value, and Windows
# line endings.
set -eu
name=$1
file=$2
tr -d '\r' < "$file" |
  sed -n "s/^[[:space:]]*\(export[[:space:]][[:space:]]*\)\{0,1\}${name}[[:space:]]*=[[:space:]]*//p" |
  tail -n 1 |
  sed -e "s/^\"\(.*\)\"[[:space:]]*\$/\1/" -e "t" \
      -e "s/^'\(.*\)'[[:space:]]*\$/\1/" -e "t" \
      -e 's/[[:space:]]#.*$//' -e 's/[[:space:]]*$//'
