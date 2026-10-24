#!/bin/bash
# usage: shot.sh out.png "ex,ey,ez,lx,ly,lz" [secs] [w h fov]
cd "$(dirname "$0")" || exit 1
./prevbin "$2" "${3:-30}" || exit 1
python3 render.py ../shaders "$1" ${4:-1280} ${5:-800} ${6:-80} | tail -${TAILN:-1}
