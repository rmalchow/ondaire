# Standalone GPIO continuity walk — drives GP5/GP6/GP7 high one at a time so you
# can meter the DAC's BCK/LRCK/DIN input pads (DC volts vs GND) and confirm each
# MCU→DAC wire carries the signal. Not part of the firmware; a bench diagnostic.
#
#   cd esp32/tools/gpio-walk
#   idf.py set-target esp32s3
#   idf.py -p /dev/ttyACM0 flash monitor
cmake_minimum_required(VERSION 3.16)
include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(gpio-walk)
