# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""PCB v1 of the doormutex light as a netlist: every part, every pin.

gen_schematic.py turns this into doormutex.kicad_sch. The JLCPCB parts come
from lib/doormutex.* (imported with easyeda2kicad from their LCSC numbers,
so symbol, footprint and placement match what JLCPCB assembles); generic
resistors and capacitors use the KiCad library.

Blocks, top to bottom of the power path:

  USB-C ─► bq24074 (power path) ─► SYS ─┬─► TPS63802 ─► 3V3 ─► ESP32-C6, sensors, e-paper
                  ▲                     │      ▲ EN from TPS3840: off below 3.0 V on SYS
                  │ VBAT                └─► LEDs (resistor + N-MOSFET per lamp)
  18650 ─ 0 Ω (measure) ─ PTC ─ P-MOSFET (reverse polarity) ─┘
     └─ minus: DW01A + FS8205A ─► P_N ─ 20 mΩ ─► GND   (MAX17055 counts across it)
"""

# Generic parts: (lib_id, footprint, LCSC by value)
R0402 = ("Device:R", "Resistor_SMD:R_0402_1005Metric")
R0603 = ("Device:R", "Resistor_SMD:R_0603_1608Metric")
C0402 = ("Device:C", "Capacitor_SMD:C_0402_1005Metric")
C0603 = ("Device:C", "Capacitor_SMD:C_0603_1608Metric")
C0805 = ("Device:C", "Capacitor_SMD:C_0805_2012Metric")

LCSC = {
    # resistors (JLCPCB basic unless noted)
    "5.1k": "C25905", "10k": "C25744", "100k": "C25741", "4.7k": "C25900", "100": "C25076", "1k": "C11702",
    "10M": "C7250",  # 0603
    "887": "C474137",  # ext
    "1.1k": "C25860",  # ext
    "511k": "C64042",  # ext
    "91k": "C4147",  # ext
    "68": "C25131",  # ext
    "150": "C25082",  # ext, preferred
    # capacitors
    "100n": "C1525", "1u": "C52923", "470n": "C47339", "4.7u": "C19666", "10u": "C19702", "22u": "C59461",
    "47u": "C16780",  # 0805
    "10p": "C32949",
}

parts = []


def part(ref, lib_id, value, pins, footprint="", lcsc="", dnp=False, block=""):
    parts.append(dict(ref=ref, lib_id=lib_id, value=value, pins=pins, footprint=footprint, lcsc=lcsc, dnp=dnp, block=block))


def jlc(ref, symbol, value, lcsc, pins, block, dnp=False):
    part(ref, "doormutex:" + symbol, value, pins, lcsc=lcsc, dnp=dnp, block=block)


def r(ref, value, a, b, block, pkg=R0402, dnp=False):
    part(ref, pkg[0], value, {"1": a, "2": b}, pkg[1], LCSC.get(value, ""), dnp, block)


def c(ref, value, a, b, block, pkg=C0402, dnp=False):
    part(ref, pkg[0], value, {"1": a, "2": b}, pkg[1], LCSC.get(value, ""), dnp, block)


NC = None  # an unconnected pin gets a no-connect flag

# ---- USB-C, ESD
B = "USB"
jlc("J1", "TYPE-C-31-M-12", "USB-C", "C165948", {
    "A4B9": "VBUS", "B4A9": "VBUS", "A1B12": "GND", "B1A12": "GND",
    "A5": "CC1", "B5": "CC2", "A6": "USB_DP", "B6": "USB_DP", "A7": "USB_DN", "B7": "USB_DN",
    "A8": NC, "B8": NC, "1": "GND", "2": "GND", "3": "GND", "4": "GND"}, B)
r("R1", "5.1k", "CC1", "GND", B)
r("R2", "5.1k", "CC2", "GND", B)
jlc("U1", "USBLC6-2SC6", "USBLC6-2SC6", "C7519", {
    "1": "USB_DP", "6": "USB_DP", "3": "USB_DN", "4": "USB_DN", "2": "GND", "5": "VBUS"}, B)

# ---- charger with power path
B = "Charger"
jlc("U2", "BQ24074RGTR", "BQ24074", "C54313", {
    "13": "VBUS", "2": "VBAT", "3": "VBAT", "10": "SYS", "11": "SYS", "8": "GND", "17": "GND",
    "16": "ISET", "12": "ILIM",
    "6": "GND",   # EN1 low + EN2 high: input current limit set by ILIM
    "5": "SYS",
    "4": "GND",   # /CE: charging always enabled
    "14": NC,     # TMR open: default safety timer
    "15": NC,     # ITERM open: 10 % termination
    "1": "TS", "9": "CHG", "7": "PGOOD"}, B)
r("R3", "887", "ISET", "GND", B)   # 890 AΩ / 887 Ω = 1.0 A charge current
r("R4", "1.1k", "ILIM", "GND", B)  # 1610 AΩ / 1.1 kΩ = 1.46 A input limit
c("C1", "1u", "VBUS", "GND", B)
c("C2", "10u", "VBUS", "GND", B, C0603)
c("C3", "10u", "SYS", "GND", B, C0603)
c("C4", "4.7u", "VBAT", "GND", B, C0603)
# NTC at the cell holder: charging only between about 0 and 50 °C
jlc("TH1", "NCP18XH103F03RB", "10k NTC", "C13564", {"1": "TS", "2": "GND"}, B)

# ---- cell, protection, fuel gauge
B = "Cell"
jlc("BT1", "1042", "18650", "C5249310", {"2": "CELL_P", "1": "CELL_N"}, B)
# 0 Ω in the cell's plus lead: lift it to measure the current (or solder J3)
jlc("R5", "1206W4F0000T5E", "0", "C17888", {"1": "CELL_P", "2": "CELL_PM"}, B)
part("J3", "Connector_Generic:Conn_01x02", "measure", {"1": "CELL_P", "2": "CELL_PM"},
     "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical", dnp=True, block=B)
jlc("F1", "1812L200THPR", "PTC 2 A", "C207074", {"1": "CELL_PM", "2": "CELL_PF"}, B)
# Reverse polarity: the body diode conducts until the gate (at the cell's
# minus) turns the channel on; with the cell the wrong way round it blocks.
jlc("Q1", "DMP2035U-7", "DMP2035U", "C110499", {"3": "CELL_PF", "2": "VBAT", "1": "Q1_G"}, B)
r("R6", "10k", "Q1_G", "CELL_N", B)
# DW01A + dual N-MOSFET in the minus lead (4.30 V / 2.50 V, short ~3 A)
jlc("U3", "DW01A_C351410", "DW01A", "C351410", {
    "5": "DW_VCC", "6": "CELL_N", "2": "DW_VM", "1": "DW_OD", "3": "DW_OC", "4": NC}, B)
r("R7", "100", "VBAT", "DW_VCC", B)
c("C5", "100n", "DW_VCC", "CELL_N", B)
r("R8", "1k", "DW_VM", "P_N", B)
jlc("Q2", "FS8205A_C908265", "FS8205A", "C908265", {
    "1": "CELL_N", "3": "P_N", "2": "FET_D", "5": "FET_D", "6": "DW_OD", "4": "DW_OC"}, B)
# Fuel gauge: its ground is the battery side of the sense resistor
jlc("R9", "RLM10FTSMR020", "20m", "C163095", {"1": "P_N", "2": "GND"}, B)
jlc("U4", "MAX17055ETB+T", "MAX17055", "C2976422", {
    "10": "VBAT", "6": "P_N", "11": "P_N", "5": "GND", "7": "GAUGE_REG",
    "3": "SCL", "4": "SDA", "8": "GAUGE_ALRT", "2": "P_N", "9": NC, "1": NC}, B)
c("C6", "100n", "VBAT", "P_N", B)
c("C7", "470n", "GAUGE_REG", "P_N", B)

# ---- under-voltage cut-off and 3.3 V
B = "3V3"
# TPS3840: open-drain /RESET low below 3.0 V on SYS (100 mV hysteresis) keeps
# the buck-boost off; the RC keeps Wi-Fi current dips from tripping it.
jlc("U5", "TPS3840DL30DBVR", "TPS3840DL30", "C2862542", {
    "2": "SV_VDD", "3": "GND", "1": "EN_3V3", "4": NC, "5": NC}, B)
r("R10", "10k", "SYS", "SV_VDD", B)
c("C8", "1u", "SV_VDD", "GND", B)
r("R11", "10M", "SYS", "EN_3V3", B, R0603)
jlc("U6", "TPS63802DLAR", "TPS63802", "C2845237", {
    "10": "SYS", "1": "EN_3V3", "2": "GND", "3": "GND", "8": "GND",
    "9": "SW1", "7": "SW2", "6": "3V3", "4": "FB", "5": NC}, B)
jlc("L1", "DFE201612E-R47M=P2", "0.47u", "C668312", {"1": "SW1", "2": "SW2"}, B)
r("R12", "511k", "3V3", "FB", B)  # 0.5 V × (1 + 511/91) = 3.31 V
r("R13", "91k", "FB", "GND", B)
c("C9", "10u", "SYS", "GND", B, C0603)
c("C10", "22u", "3V3", "GND", B, C0603)
c("C11", "22u", "3V3", "GND", B, C0603)

# ---- ESP32-C6
B = "MCU"
mcu = {"3": "3V3", "8": "EN", "23": "BOOT", "22": "IO8",
       "17": "USB_DN", "18": "USB_DP",
       "5": "LED_R", "6": "LED_Y", "15": "LED_G",
       "16": "CHG", "30": "PGOOD", "31": "TXD0",
       "28": "SDA", "29": "SCL",
       "24": "EPD_DIN", "25": "EPD_CLK", "26": "EPD_CS", "27": "EPD_DC", "19": "EPD_RST", "20": "EPD_BUSY",
       "10": "INFO", "9": "GAUGE_ALRT", "12": "XTAL_P", "13": "XTAL_N",
       "4": NC, "7": NC, "21": NC, "32": NC, "33": NC, "34": NC, "35": NC}
for g in [1, 2, 11, 14] + list(range(36, 54)):
    mcu[str(g)] = "GND"
jlc("U7", "ESP32-C6-MINI-1-H8", "ESP32-C6-MINI-1-H8", "C42403058", mcu, B)
c("C12", "47u", "3V3", "GND", B, C0805)
c("C13", "100n", "3V3", "GND", B)
r("R14", "10k", "3V3", "EN", B)
c("C14", "1u", "EN", "GND", B)
r("R15", "10k", "3V3", "BOOT", B)
r("R16", "10k", "3V3", "IO8", B)   # strapping: GPIO8 high at boot
jlc("SW1", "TS-1187A-B-A-B", "RESET", "C318884", {"1": "EN", "2": "EN", "3": "GND", "4": "GND"}, B)
jlc("SW2", "TS-1187A-B-A-B", "BOOT", "C318884", {"1": "BOOT", "2": "BOOT", "3": "GND", "4": "GND"}, B)
# Info button at the bottom edge next to USB-C, on an LP pin (wakes from deep sleep)
jlc("SW3", "SKRTLAE010", "INFO", "C110293", {"1": "INFO", "3": "INFO", "2": "GND", "4": "GND", "5": "GND"}, B)
r("R17", "10k", "3V3", "INFO", B)
r("R18", "10k", "3V3", "GAUGE_ALRT", B)
# 32.768 kHz crystal for the RTC – footprint only, not fitted in v1
part("Y1", "Device:Crystal", "32.768k", {"1": "XTAL_P", "2": "XTAL_N"},
     "Crystal:Crystal_SMD_3215-2Pin_3.2x1.5mm", dnp=True, block=B)
c("C15", "10p", "XTAL_P", "GND", B, dnp=True)
c("C16", "10p", "XTAL_N", "GND", B, dnp=True)

# ---- I²C: light sensor (0x10), fuel gauge (0x36)
B = "Sensors"
r("R19", "4.7k", "3V3", "SDA", B)
r("R20", "4.7k", "3V3", "SCL", B)
jlc("U8", "VEML7700-TR", "VEML7700", "C504893", {"2": "3V3", "3": "GND", "1": "SCL", "4": "SDA"}, B)
c("C17", "100n", "3V3", "GND", B)

# ---- e-paper: the Waveshare 1.54″ module on an 8-pin socket
B = "E-paper"
jlc("J2", "PM2.54-1X8PM-H85", "e-paper", "C42431840", {
    "1": "3V3", "2": "GND", "3": "EPD_DIN", "4": "EPD_CLK", "5": "EPD_CS", "6": "EPD_DC",
    "7": "EPD_RST", "8": "EPD_BUSY"}, B)
c("C18", "100n", "3V3", "GND", B)

# ---- lamps: per colour an N-MOSFET and two LED places, the second not
# fitted (more light without a new board). From SYS: the firmware evens out
# the falling cell voltage (LED reference voltage in menuconfig).
B = "LEDs"
lamps = [  # colour, GPIO net, LED symbol, LCSC, series resistor
    ("R", "LED_R", "XL-2012SURC-FJ", "C7371909", "150"),
    ("Y", "LED_Y", "XL-2012UYC-FJ", "C7371908", "150"),
    ("G", "LED_G", "XL-2012UGC", "C965815", "68"),
]
n = 21
for i, (col, gpio, sym, lcsc, rs) in enumerate(lamps):
    q = f"Q{3 + i}"
    jlc(q, "AO3400A", "AO3400A", "C20917", {"1": f"{gpio}_GATE", "2": "GND", "3": f"{gpio}_K"}, B)
    r(f"R{n}", "100", gpio, f"{gpio}_GATE", B)
    r(f"R{n + 1}", "100k", f"{gpio}_GATE", "GND", B)
    for k in (1, 2):
        anode, ref = f"{gpio}_A{k}", f"D{2 * i + k}"
        r(f"R{n + 1 + k}", rs, "SYS", anode, B, dnp=(k == 2))
        # by pin name: the three LED symbols number A and K differently
        jlc(ref, sym, f"LED {col}", lcsc, {"A": anode, "K": f"{gpio}_K"}, B, dnp=(k == 2))
    n += 4

# ---- test points
B = "Test"
for i, net in enumerate(["VBUS", "VBAT", "SYS", "3V3", "GND", "P_N", "TXD0"], start=1):
    jlc(f"TP{i}", "RH-5015", net, "C5199798", {"1": net}, B)

# ---- PWR_FLAGs: these nets are driven by passives or connectors only
for i, net in enumerate(["VBUS", "VBAT", "SYS", "3V3", "GND", "CELL_N", "P_N"], start=1):
    part(f"#FLG{i}", "power:PWR_FLAG", "PWR_FLAG", {"1": net}, block="Power flags")
