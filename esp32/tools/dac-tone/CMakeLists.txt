# Standalone DAC wiring bench test — plays a continuous sine tone over I2S using
# the same pins/config as the ondaire player (board_esp32s3_supermini.h).
# Not part of the firmware; build/flash it to verify a PCM5102A is wired right.
#
#   cd esp32/tools/dac-tone
#   idf.py set-target esp32s3
#   idf.py -p /dev/ttyACM0 flash monitor
cmake_minimum_required(VERSION 3.16)
include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(dac-tone)
