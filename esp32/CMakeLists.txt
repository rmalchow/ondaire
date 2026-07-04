# ondaire playback-node firmware — ESP-IDF top-level project.
#
# Build for a specific board with the matching sdkconfig defaults, e.g.
#   idf.py -DBOARD=esp32s3 set-target esp32s3
#   idf.py build
# or just use ./build.sh <board>. The selected board is exposed to the
# firmware via Kconfig (ONDAIRE_BOARD_*) and the boards/ pin headers.
cmake_minimum_required(VERSION 3.16)

# Per-board sdkconfig: SDKCONFIG_DEFAULTS layers the common file plus the
# board-specific one when BOARD is passed (build.sh sets it).
if(DEFINED BOARD)
    set(SDKCONFIG_DEFAULTS "sdkconfig.defaults;sdkconfig.defaults.${BOARD}")
endif()

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(ondaire-node)
