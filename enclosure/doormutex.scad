// SPDX-FileCopyrightText: 2026 The doormutex contributors
// SPDX-License-Identifier: CERN-OHL-S-2.0
//
// doormutex enclosure v0 – a miniature traffic light for the breadboard
// build (ready-made modules, see docs/breadboard.md).
//
// Coordinates: x = width (centre 0), y = depth (front face at 0, back at
// `depth`), z = height (bottom at 0). Every printable part is exported in
// its print orientation and prints without supports.
//
// Export a part:  openscad -D 'part="front"' -o front.stl doormutex.scad
// Plain OpenSCAD, no libraries – works with the Customizer.

/* [Part] */
// What to render: an assembly view or one printable part
part = "assembly"; // [assembly, exploded, front, back, foot, side_plate, visor, diffuser]

/* [Body] */
// Outer width; the e-ink module sets the minimum
width = 45;
// Height of one light chamber
chamber_h = 45;
// Strip above the red chamber
roof_h = 5;
// Height of the sign with the e-ink display
sign_h = 52;
// Total depth incl. the back cover (breadboard modules need ~45 mm; the later PCB ~38 mm)
depth = 45;
// Side, top, bottom and back walls (multiples of the 0.45 mm line width)
wall = 1.8;
// Front wall – carries the lens seats and the display window
front_wall = 2.4;
// Chamfer on the outer edges
edge_chamfer = 1.2;

/* [Fits] */
// Sliding fit – calibrate with a test print
tol = 0.2;
// Press fit
press_tol = 0.1;

/* [Lenses and chambers] */
// Lens diameter (transparent PETG disc)
lens_d = 30;
// Lens thickness
lens_t = 1.0;
// Radial overlap of the lens on its seat
lens_seat = 1.5;
// Depth of the reflector cone behind the lens
cone_depth = 15;
// Wall of the reflector cone
cone_wall = 1.2;
// LED diameter (5 mm THT LEDs, pushed in from behind)
led_d = 5;
// Height of the diffuser rings on the inside of the lens
diffuser_rib = 0.4;
// Pitch of the diffuser rings
diffuser_pitch = 1.6;
// Walls between the chambers
divider_t = 1.2;

/* [Visors] */
// Length of the visor at the top
visor_len = 15;
// Visor wall
visor_t = 1.2;
// Angle the visor covers, centred on the top
visor_cover = 200; // [120:10:270]
// Slant of the visor's front edge (0 = straight)
visor_slant = 35; // [0:5:60]
// Thickness of the visor ring on the front face
visor_ring_t = 2.0;
// Dowels: short pieces of 1.75 mm filament
dowel_d = 1.75;

/* [E-ink module] */
// Waveshare 1.54" e-Paper Module V2, mounted rotated (W x H). TODO verify on the module
eink_module = [33, 48];
// PCB thickness of the module
eink_pcb_t = 1.6;
// Panel thickness (glued to the front of the module PCB). TODO verify
eink_panel_t = 1.2;
// Panel outline (W x H, rotated like the module). TODO verify
eink_panel = [31.8, 37.32];
// Active area of the 200 x 200 px panel
eink_active = [27.6, 27.6];
// Offset of the active area from the module centre (x, z). TODO verify
eink_active_offset = [0, 0];
// Margin of the window around the active area
eink_margin = 0.5;
// 45° bevel around the window on the front face
eink_bevel = 1.0;
// Distance of the module's mounting holes from its edges. TODO verify
eink_hole_inset = 2.5;
// Hole for M2 self-tapping screws
eink_hole_d = 1.8;

/* [Back cover and modules] */
// Screws for the back cover: heat-set inserts (M3 x 4) or self-tapping M3
cover_screw = "insert"; // [insert, self_tap]
// Size of the corner ribs that take all screws
rib = 6;
// Where the corner ribs start behind the front wall
rib_start = 12;
// ESP32-C6-DevKitC-1 board (W x H), component side towards the back cover. TODO verify
devkit = [25.4, 51.8];
// Space between back cover and DevKit (the USB-C sockets sit in it)
devkit_gap = 7;
// USB-C socket used (x offset from the board centre). TODO verify
usb_x = -6.5;
// USB-C socket: width, height
usb_socket = [8.94, 3.2];
// BOOT and RESET buttons (x, z from the board's bottom centre) – pin holes. TODO verify
devkit_buttons = [[-8, 4], [8, 4]];
// Charger Adafruit 4755 (bq24074), W x H. TODO verify
charger = [28, 40];
// Mounting hole distance from the charger's edges. TODO verify
charger_hole_inset = 2.5;
// 18650 holder with leads (W x H x D). TODO verify
holder = [21, 77, 19];
// Fuel gauge Adafruit 5580 (STEMMA QT, 1" x 0.7"), mounted upright
gauge = [17.8, 25.4];
// Pololu S7V8F3 buck-boost (glued with a foam pad)
boost = [10.2, 12.7];
// Light sensor Adafruit 4162 VEML7700 (1" x 0.7"), lying under the top wall
als = [25.4, 17.8];
// Window for the light sensor in the top wall
als_window_d = 3;
// Info button (6 x 6 mm tactile switch) on the back cover: x, z
info_button = [11, 114];
// Pin hole diameter (paper clip)
pin_hole_d = 1.6;
// Standoff height for the modules on the back cover
standoff_h = 4;
// Hole for M2.5 self-tapping screws
m25_hole_d = 2.2;

/* [Magnets] */
// Neodymium disc magnets: diameter
magnet_d = 12;
// Magnet height
magnet_h = 3;
// Clearance around the magnet (press fit: 0.1–0.2)
magnet_tol = 0.15;
// Plastic between magnet and pad
magnet_skin = 0.6;
// Embedded: pause the print and drop the magnets in (no glue). Otherwise open pockets for glue
magnet_embed = true;
// Recess for a rubber/silicone pad on the contact faces
pad_recess = 0.5;
// Magnet positions in the foot (x, y)
foot_magnets = [[-11, 13], [11, 13]];
// Screw positions of the foot (y); x is the rib centre
foot_screw_y = 20;
// Groove for a cable tie or cord around the post (y position)
tie_groove_y = 28;
// Side plate for steel railings: none, left or right
side_plate = "right"; // [none, left, right]
// Magnet positions in the side plate (y, z)
side_magnets = [[22.5, 25], [22.5, 75], [22.5, 120], [22.5, 165]];
// Screws of the side plate (y position)
side_screw_y = 30;

/* [Quality] */
$fn = 64;

// ---------------------------------------------------------------- derived

H = roof_h + 3 * chamber_h + sign_h;
iw = width - 2 * wall;                  // inner width
ih_top = H - wall;                      // inner top
back_in = depth - wall;                 // inner face of the back cover
light_d = lens_d - 2 * lens_seat;       // opening in the front wall
led_r_in = (led_d + 1.6) / 2;           // cone radius at the LED
cap_t = 1.2;                            // back end of the cone
eps = 0.01;

lamp_colors = ["#1fa84f", "#f5b301", "#e5322d"];
shell_color = "#3a3d44";

function chamber_z(i) = sign_h + chamber_h * (i + 0.5); // 0 green, 1 yellow, 2 red
eink_z = (wall + sign_h - divider_t / 2) / 2;
rib_x = iw / 2 - rib / 2;               // rib centre, |x|
rib_z = [wall + rib / 2, ih_top - rib / 2];

// The DevKit sits centred above the bottom wall, component side towards the cover.
dk_z0 = wall + 0.3;
dk_back = back_in - devkit_gap;          // component side of the PCB
dk_front = dk_back - 1.6;
usb_y = dk_back + usb_socket[1] / 2;

charger_z = [dk_z0 + devkit[1] + 3, dk_z0 + devkit[1] + 3 + charger[1]];
holder_z = [ih_top - 10 - holder[1], ih_top - 10];
holder_x = [-iw / 2 + 0.3, -iw / 2 + 0.3 + holder[0]];
gauge_x = [iw / 2 - gauge[0], iw / 2];
gauge_z = [holder_z[1] - gauge[1], holder_z[1]];
boost_pos = [11, 146];                   // centre x, z
als_clear = 4;                           // sensor board + parts under the top wall
als_reach = 20;                          // bracket depth from the cover
als_zb = ih_top - als_clear;             // top of the bracket
als_y = back_in - als_reach / 2;

foot_t = pad_recess + magnet_skin + magnet_h + magnet_tol + (magnet_embed ? 0.6 : 0);

cover_hole_d = cover_screw == "insert" ? 4.0 : 2.6;
cover_hole_len = cover_screw == "insert" ? 6.5 : 8;   // inserts e.g. M3 x 5.7 mm

// ---------------------------------------------------------------- helpers

module along_y() rotate([-90, 0, 0]) children();   // +z → +y

// Box with all twelve edges chamfered; origin at the min corner.
module chamfered_box(s, c) {
  hull() {
    translate([c, 0, c]) cube([s.x - 2 * c, s.y, s.z - 2 * c]);
    translate([0, c, c]) cube([s.x, s.y - 2 * c, s.z - 2 * c]);
    translate([c, c, 0]) cube([s.x - 2 * c, s.y - 2 * c, s.z]);
  }
}

// Plate with chamfered vertical edges; origin at the min corner.
module chamfered_plate(s, t, c) hull() {
  translate([c, 0, 0]) cube([s.x - 2 * c, s.y, t]);
  translate([0, c, 0]) cube([s.x, s.y - 2 * c, t]);
}

// Teardrop profile with its tip towards +y – horizontal holes print without supports.
module teardrop2d(d) hull() { circle(d = d); translate([0, d / 2 * sqrt(2)]) square(0.01, center = true); }

// Countersunk screw hole along +z, head at z = 0.
module countersink(d_hole, d_head, len) {
  translate([0, 0, -eps]) cylinder(d = d_hole, h = len);
  translate([0, 0, -eps]) cylinder(d1 = d_head, d2 = d_hole, h = (d_head - d_hole) / 2 + eps);
}

module outer() translate([-width / 2, 0, 0]) chamfered_box([width, depth, H], edge_chamfer);

// ---------------------------------------------------------------- front shell

// Reflector cone: light-tight from the lens to the LED.
module chamber(i) {
  translate([0, front_wall, chamber_z(i)]) along_y() difference() {
    cylinder(h = cone_depth + cap_t, r1 = light_d / 2 + cone_wall, r2 = led_r_in + cone_wall);
    translate([0, 0, -eps]) cylinder(h = cone_depth + eps, r1 = light_d / 2, r2 = led_r_in);
    cylinder(h = 3 * cone_depth, d = led_d + 2 * press_tol, center = true);
  }
}

module dividers()
  for (k = [0 : 3])
    translate([-iw / 2, front_wall - eps, sign_h + k * chamber_h - divider_t / 2])
      cube([iw, cone_depth + cap_t, divider_t]);

// Ribs in the four long corners; they take the cover, foot and side screws.
// The front end is a 45° wedge, so it prints without supports.
module corner_ribs()
  for (sx = [-1, 1], top = [0, 1]) {
    x0 = sx > 0 ? iw / 2 - rib : -iw / 2;
    xw = sx > 0 ? iw / 2 - eps : -iw / 2;
    z0 = top ? ih_top - rib : wall;
    hull() {
      translate([x0, rib_start, z0]) cube([rib, back_in - rib_start, rib]);
      translate([xw, rib_start - rib, z0]) cube([eps, back_in - rib_start + rib, rib]);
    }
  }

module eink_holes(d, h)
  for (sx = [-1, 1], sz = [-1, 1])
    translate([sx * (eink_module[0] / 2 - eink_hole_inset), 0, eink_z + sz * (eink_module[1] / 2 - eink_hole_inset)])
      along_y() cylinder(d = d, h = h);

module eink_standoffs() translate([0, front_wall - eps, 0]) eink_holes(4.5, eink_panel_t + 0.2);

module eink_window() {
  a = eink_active + [2, 2] * eink_margin;
  c = [eink_active_offset[0], eink_z + eink_active_offset[1]];
  translate([c[0] - a[0] / 2, -1, c[1] - a[1] / 2]) cube([a[0], front_wall + 2, a[1]]);
  hull() {
    translate([c[0] - a[0] / 2 - eink_bevel, -1, c[1] - a[1] / 2 - eink_bevel]) cube([a[0] + 2 * eink_bevel, 1, a[1] + 2 * eink_bevel]);
    translate([c[0] - a[0] / 2, 0, c[1] - a[1] / 2]) cube([a[0], eink_bevel, a[1]]);
  }
}

module lens_cuts()
  for (i = [0 : 2]) translate([0, 0, chamber_z(i)]) {
    translate([0, -1, 0]) along_y() cylinder(d = light_d, h = front_wall + 2);
    // lens seat: flush with the front face, the visor ring clamps the lens
    translate([0, -1, 0]) along_y() cylinder(d = lens_d + 2 * tol, h = 1 + lens_t);
    for (sx = [-1, 1]) translate([sx * (lens_d / 2 + 3), -1, 0]) along_y() cylinder(d = dowel_d + 0.15, h = 1 + 1.5);
  }

module usb_cut() {
  slot = usb_socket + [2, 2] * (tol + 0.3);
  translate([usb_x - slot[0] / 2, usb_y - slot[1] / 2, -1]) cube([slot[0], slot[1], wall + 2]);
  // Recess for the plug's overmould, so the plug reaches deep enough;
  // its back side is a 45° slope instead of a bridge.
  r = [13, 7.5]; rd = 1.0;
  hull() {
    translate([usb_x - r[0] / 2, usb_y - r[1] / 2, -1]) cube([r[0], r[1], 1]);
    translate([usb_x - r[0] / 2, usb_y - r[1] / 2, -1]) cube([r[0], r[1] - rd, 1 + rd]);
  }
}

module rib_holes() {
  // back cover screws, along y
  for (sx = [-1, 1], z = rib_z)
    translate([sx * rib_x, back_in - cover_hole_len, z]) along_y() cylinder(d = cover_hole_d, h = cover_hole_len + 1);
  // foot screws from below, along z (teardrop: horizontal when printed)
  for (sx = [-1, 1]) translate([sx * rib_x, foot_screw_y, -1]) linear_extrude(wall + rib) teardrop2d(2.6);
  // side plate screws, along x, through both side walls
  for (sx = [-1, 1], z = rib_z)
    translate([sx * (width / 2 + 1), side_screw_y, z]) rotate([0, sx > 0 ? -90 : 90, 0])
      linear_extrude(wall + rib) teardrop2d(2.6);
}

module als_window() translate([0, als_y, ih_top - 1]) linear_extrude(wall + 2) teardrop2d(als_window_d);

module front() {
  color(shell_color) difference() {
    union() {
      difference() {
        intersection() { outer(); translate([-width, -1, -1]) cube([2 * width, back_in + 1, H + 2]); }
        translate([-iw / 2, front_wall, wall]) cube([iw, depth, H - 2 * wall]);
      }
      intersection() {
        translate([-iw / 2 - eps, front_wall - eps, wall - eps]) cube([iw + 2 * eps, back_in - front_wall + eps, H - 2 * wall + 2 * eps]);
        union() {
          for (i = [0 : 2]) chamber(i);
          dividers();
          corner_ribs();
          eink_standoffs();
        }
      }
    }
    lens_cuts();
    eink_window();
    translate([0, 0.9, 0]) eink_holes(eink_hole_d, front_wall + eink_panel_t + 1);
    usb_cut();
    rib_holes();
    als_window();
  }
}

// ---------------------------------------------------------------- back cover

// Locating tabs instead of a full rim – the cell holder and the gauge sit at the walls.
module cover_tabs() {
  t = 1.2; h = 3; l = 10;
  for (p = [[-1, 30], [-1, 95], [1, 30], [1, 130]])
    translate([p[0] > 0 ? iw / 2 - tol - t : -iw / 2 + tol, back_in - h, p[1] - l / 2]) cube([t, h + eps, l]);
  for (z = [wall + tol, ih_top - tol - t])
    translate([-l / 2, back_in - h, z]) cube([l, h + eps, t]);
}

// A standoff from the inner face of the cover towards the front.
module standoff(x, z, d = 5, h = standoff_h) translate([x, back_in - h, z]) along_y() cylinder(d = d, h = h + eps);
module standoff_hole(x, z, h = standoff_h) translate([x, back_in - h - 1, z]) along_y() cylinder(d = m25_hole_d, h = h + 1 + wall - 0.6);

function charger_holes() = [for (sx = [-1, 1], sz = [0, 1])
  [sx * (charger[0] / 2 - charger_hole_inset), charger_z[sz] + (sz ? -1 : 1) * charger_hole_inset]];
function gauge_holes() = [for (sz = [-1, 1]) [(gauge_x[0] + gauge_x[1]) / 2, (gauge_z[0] + gauge_z[1]) / 2 + sz * (gauge[1] / 2 - 2.54)]];

// DevKit: four posts (the lower pair above the USB-C sockets), guides along
// the long edges and a snap lip at the top edge – the pin headers run along
// the long edges, the top end (antenna) is bare on the pin side.
module devkit_mount() {
  w = devkit[0] + 2 * tol;
  for (sx = [-1, 1], z = [dk_z0 + 9, dk_z0 + devkit[1] - 1.5])
    translate([sx * (devkit[0] / 2 - 1.5), dk_back, z]) along_y() cylinder(d = 3, h = devkit_gap + eps);
  for (sx = [-1, 1], z = [dk_z0 + devkit[1] / 4, dk_z0 + devkit[1] * 3 / 4])
    translate([sx > 0 ? w / 2 : -w / 2 - 1.2, dk_front - 1, z - 4]) cube([1.2, back_in - dk_front + 1 + eps, 8]);
  zt = dk_z0 + devkit[1] + tol;
  translate([-4, dk_front - 1.4, zt]) cube([8, back_in - dk_front + 1.4 + eps, 1.2]);
  // lip: a 45° diamond, so both snapping in and printing work
  translate([4, 0, 0]) rotate([0, -90, 0]) linear_extrude(8)
    polygon([[zt, dk_front - 0.05], [zt - 0.6, dk_front - 0.65], [zt, dk_front - 1.25]]);
}

module holder_cradle() {
  // the shell wall guides the left side; a low wall on the right, guides at top and bottom
  translate([holder_x[1] + tol, back_in - 4, holder_z[0]]) cube([1.2, 4 + eps, holder[1]]);
  for (z = [holder_z[0] - tol - 1.2, holder_z[1] + tol])
    translate([holder_x[0], back_in - 4, z]) cube([holder[0], 4 + eps, 1.2]);
}

module als_bracket() {
  w = 22;
  difference() {
    translate([-w / 2, back_in - als_reach, als_zb - 2.4]) cube([w, als_reach + eps, 2.4]);
    for (sx = [-1, 1]) translate([sx * (als[0] / 2 - 2.54), als_y, als_zb - 3])
      linear_extrude(4) mirror([0, 1, 0]) teardrop2d(m25_hole_d);
  }
}

module button_frame() {
  s = 6.2;
  translate([info_button[0], back_in - 2.5, info_button[1]]) difference() {
    translate([-s / 2 - 1.2, 0, -s / 2 - 1.2]) cube([s + 2.4, 2.5 + eps, s + 2.4]);
    translate([-s / 2, -1, -s / 2]) cube([s, 4, s]);
  }
}

module back() {
  color(shell_color) difference() {
    union() {
      intersection() { outer(); translate([-width, back_in, -1]) cube([2 * width, wall + 1, H + 2]); }
      cover_tabs();
      devkit_mount();
      for (p = charger_holes()) standoff(p[0], p[1]);
      for (p = gauge_holes()) standoff(p[0], p[1]);
      holder_cradle();
      als_bracket();
      button_frame();
      // marks the spot for the buck-boost (foam pad)
      translate([boost_pos[0] - boost[0] / 2 - 0.8, back_in - 0.6, boost_pos[1] - boost[1] / 2 - 0.8])
        difference() { cube([boost[0] + 1.6, 0.6 + eps, boost[1] + 1.6]); translate([0.8, -1, 0.8]) cube([boost[0], 3, boost[1]]); }
    }
    for (sx = [-1, 1], z = rib_z)
      translate([sx * rib_x, depth, z]) rotate([90, 0, 0]) countersink(3.4, 6.4, wall + 1);
    for (p = charger_holes()) standoff_hole(p[0], p[1]);
    for (p = gauge_holes()) standoff_hole(p[0], p[1]);
    for (b = concat(devkit_buttons, [[info_button[0], info_button[1] - dk_z0]]))
      translate([b[0], back_in - 3, dk_z0 + b[1]]) along_y() cylinder(d = pin_hole_d, h = wall + 4);
    // slots for a velcro strap around the cell holder
    for (x = [holder_x[0] + 1.6, holder_x[1] - 1.6], z = [holder_z[0] + 18, holder_z[1] - 18])
      translate([x - 1.25, back_in - 1, z - 7]) cube([2.5, wall + 2, 14]);
  }
}

// ---------------------------------------------------------------- lens, visor

// Lens: smooth side on the bed / outside, concentric rings inside spread the LED's hot spot.
module diffuser_body() {
  cylinder(d = lens_d, h = lens_t);
  for (r = [diffuser_pitch : diffuser_pitch : light_d / 2 - 0.5])
    translate([0, 0, lens_t - eps]) difference() {
      cylinder(r = r + 0.225, h = diffuser_rib + eps);
      translate([0, 0, -1]) cylinder(r = r - 0.225, h = 3);
    }
}

// Visor in local coordinates: ring back face at y = 0, hood towards -y.
module visor_local() {
  ro = lens_d / 2 + 6;
  ri = lens_d / 2 - 1;
  hr_in = lens_d / 2 + 0.5;
  hr_out = hr_in + visor_t;
  a0 = 90 - visor_cover / 2;
  sector = concat([[0, 0]], [for (a = [a0 : (visor_cover / 36) : 90 + visor_cover / 2 + eps]) 2 * ro * [cos(a), sin(a)]]);
  color(shell_color) difference() {
    union() {
      translate([0, -visor_ring_t, 0]) along_y() cylinder(r = ro, h = visor_ring_t);
      // hood: part of a tube around the top, its front edge slanted
      intersection() {
        rotate([90, 0, 0]) linear_extrude(visor_len + visor_ring_t)
          intersection() { difference() { circle(r = hr_out); circle(r = hr_in); } polygon(sector); }
        translate([0, -visor_ring_t - visor_len, hr_out]) rotate([visor_slant, 0, 0]) translate([-2 * ro, 0, -200]) cube([4 * ro, 200, 400]);
      }
    }
    translate([0, -visor_ring_t - 1, 0]) along_y() cylinder(r = ri, h = visor_ring_t + 2);
    for (sx = [-1, 1]) translate([sx * (lens_d / 2 + 3), -1.5, 0]) along_y() cylinder(d = dowel_d + 0.15, h = 2);
  }
}

// ---------------------------------------------------------------- foot, side plate

module magnet_pocket(open)
  translate([0, 0, pad_recess + magnet_skin]) cylinder(d = magnet_d + 2 * magnet_tol, h = magnet_h + magnet_tol + (open ? 10 : 0));

module pad_recess_cut(s) translate([2, 2, -1]) cube([s.x - 4, s.y - 4, 1 + pad_recess]);

// Foot in local coordinates: contact face at z = 0, body on top at z = foot_t.
module foot_local() {
  s = [width, depth];
  color(shell_color) difference() {
    translate([-width / 2, 0, 0]) chamfered_plate(s, foot_t, edge_chamfer);
    translate([-width / 2, 0, 0]) pad_recess_cut(s);
    for (p = foot_magnets) translate([p[0], p[1], 0]) magnet_pocket(!magnet_embed);
    for (sx = [-1, 1]) translate([sx * rib_x, foot_screw_y, 0]) countersink(3.4, 6.4, foot_t + 1);
    // USB-C stays reachable: slot open to the bottom and the back
    translate([usb_x - 7, usb_y - 4.5, -1]) cube([14, depth, foot_t + 2]);
    // groove for a cable tie or a cord around the post – a falling light is dangerous
    translate([-width, tie_groove_y - 2.5, foot_t - 2.2]) cube([2 * width, 5, 3]);
  }
}

// Side plate in local coordinates (x = body y, y = body z): contact face at z = 0, shell side on top.
module side_plate_local() {
  s = [depth - 2 * edge_chamfer, H - 2 * edge_chamfer];
  color(shell_color) difference() {
    translate([edge_chamfer, edge_chamfer, 0]) chamfered_plate(s, foot_t, edge_chamfer);
    translate([edge_chamfer, edge_chamfer, 0]) pad_recess_cut(s);
    for (p = side_magnets) translate([p[0], p[1], 0]) magnet_pocket(!magnet_embed);
    for (z = rib_z) translate([side_screw_y, z, 0]) countersink(3.4, 6.4, foot_t + 1);
  }
}

// ---------------------------------------------------------------- dummies (assembly views only)

// Boxes and cylinders shrunk by $g on every side – the collision check
// uses $g > 0, so faces that merely touch don't count.
$g = 0;
module box(p, s) translate(p + [$g, $g, $g]) cube(s - 2 * [$g, $g, $g]);
module cyl(d, h) translate([0, 0, $g]) cylinder(d = d - 2 * $g, h = h - 2 * $g);

module dummies() {
  // LEDs, flange against the back of the cone
  for (i = [0 : 2]) color(lamp_colors[i]) translate([0, front_wall + cone_depth + cap_t + 1, chamber_z(i)]) rotate([90, 0, 0]) {
    cyl(led_d, 8.6);
    cyl(5.8, 1);
  }
  // e-ink module
  translate([0, front_wall, eink_z]) {
    color("#e8e8e0") box([-eink_panel[0] / 2, 0, -eink_panel[1] / 2], [eink_panel[0], eink_panel_t, eink_panel[1]]);
    color("#1d4f8f") box([-eink_module[0] / 2, eink_panel_t + 0.2, -eink_module[1] / 2], [eink_module[0], eink_pcb_t, eink_module[1]]);
  }
  // DevKit with USB-C socket and pin headers
  color("#20304a") box([-devkit[0] / 2, dk_front, dk_z0], [devkit[0], 1.6, devkit[1]]);
  color("silver") box([usb_x - usb_socket[0] / 2, dk_back, dk_z0], [usb_socket[0], usb_socket[1], 7.3]);
  color("#222") for (sx = [-1, 1]) box([sx * (devkit[0] / 2 - 1.27) - 1.27, dk_front - 8.5, dk_z0 + 3], [2.54, 8.5, devkit[1] - 6]);
  // charger, gauge, light sensor, buck-boost
  color("#1a1a1a") box([-charger[0] / 2, back_in - standoff_h - 1.6, charger_z[0]], [charger[0], 1.6, charger[1]]);
  color("#1a1a1a") box([gauge_x[0], back_in - standoff_h - 1.6, gauge_z[0]], [gauge[0], 1.6, gauge[1]]);
  color("#1a1a1a") box([-als[0] / 2, als_y - als[1] / 2, als_zb], [als[0], als[1], 1.6]);
  color("#2a6a2a") box([boost_pos[0] - boost[0] / 2, back_in - 2.6, boost_pos[1] - boost[1] / 2], [boost[0], 2, boost[1]]);
  // 18650 holder with cell
  translate([holder_x[0], back_in - holder[2], holder_z[0]]) {
    color("#111") box([0, 0, 0], [holder[0], holder[2], holder[1]]);
    color("#3a7bd5") translate([holder[0] / 2, holder[2] / 2 - 1, 5.9]) cyl(18.4, 65.2);
  }
}

// ---------------------------------------------------------------- views and print orientation

module visors_placed(dy = 0) for (i = [0 : 2]) translate([0, dy, chamber_z(i)]) visor_local();
module lenses_placed(dy = 0)
  for (i = [0 : 2]) color(lamp_colors[i], 0.6) translate([0, dy, chamber_z(i)]) along_y() diffuser_body();
module foot_placed(dz = 0) translate([0, 0, -foot_t + dz]) foot_local();
module side_plate_placed(dx = 0)
  if (side_plate != "none") {
    sx = side_plate == "right" ? 1 : -1;
    // local (x, y, z) → body (y, z, outwards in x)
    translate([sx * (width / 2 + foot_t + dx), 0, 0])
      multmatrix([[0, 0, -sx, 0], [1, 0, 0, 0], [0, 1, 0, 0], [0, 0, 0, 1]]) side_plate_local();
  }

module view(e) {
  front();
  translate([0, 40 * e, 0]) back();
  if (e == 0) %dummies(); else dummies();
  visors_placed(-30 * e);
  lenses_placed(-15 * e);
  foot_placed(-25 * e);
  side_plate_placed(25 * e);
}

// Everything where the module dummies overlap the printed parts – must be
// empty (render.sh checks it). Faces that only touch don't count.
module collisions() {
  intersection() {
    union() { front(); back(); visors_placed(); lenses_placed(); foot_placed(); side_plate_placed(); }
    dummies($g = 0.05);
  }
  // parts against each other, moved apart by 0.05 mm (they touch by design)
  g = 0.05;
  intersection() { front(); translate([0, g, 0]) back(); }
  intersection() {
    union() { front(); translate([0, g, 0]) back(); }
    union() { visors_placed(-g); lenses_placed(-g / 2); foot_placed(-g); side_plate_placed(g); }
  }
  intersection() { visors_placed(-g); lenses_placed(-g / 2); }
}

if (part == "assembly") view(0);
else if (part == "collisions") collisions();
else if (part == "exploded") view(1);
else if (part == "front") rotate([90, 0, 0]) front();                           // front face on the bed
else if (part == "back") translate([0, 0, depth]) rotate([-90, 0, 0]) back();  // outer face on the bed
else if (part == "foot") foot_local();                                          // contact face on the bed
else if (part == "side_plate") side_plate_local();                              // contact face on the bed
else if (part == "visor") rotate([-90, 0, 0]) visor_local();                    // ring on the bed, hood up
else if (part == "diffuser") diffuser_body();                                   // smooth side on the bed
