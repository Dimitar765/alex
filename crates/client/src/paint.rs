//! Card face and back textures painted at runtime with `image` +
//! `ab_glyph`, mirroring hand3d.js's paintFront/paintBack: gradient
//! plate, gold hairline, radial art panel with the emblem PNG, shrinking
//! name, italic flavor, cost chip; the back carries the meander key
//! pattern border and the alpha.

use std::collections::HashMap;

use ab_glyph::{Font as _, FontArc, PxScale, ScaleFont as _};
use bevy::prelude::*;
use bevy::render::render_resource::{Extent3d, TextureDimension, TextureFormat};
pub const FACE_W: u32 = 512;
pub const FACE_H: u32 = 716;

const GOLD: [u8; 4] = [201, 162, 39, 255];
const GOLD_SOFT: [u8; 4] = [201, 162, 39, 140]; // 0.55 alpha
const GOLD_KEY: [u8; 4] = [201, 162, 39, 102]; // 0.4 alpha
const GOLD_ALPHA: [u8; 4] = [201, 162, 39, 217]; // 0.85 alpha
const BORDER: [u8; 4] = [74, 63, 46, 255];
const MUTED: [u8; 4] = [154, 143, 125, 255];

type Scaled = ab_glyph::PxScaleFont<FontArc>;

/// Cached painted textures: per-card fronts plus the shared back.
#[derive(Resource, Default)]
pub struct PaintedCards {
    pub fronts: HashMap<String, Handle<Image>>,
    pub back: Option<Handle<Image>>,
    emblems: HashMap<String, Option<image::RgbaImage>>,
    name_font: Option<FontArc>,
    italic_font: Option<FontArc>,
}

impl PaintedCards {
    /// Loads fonts and the shared back texture.
    pub fn boot(&mut self, images: &mut Assets<Image>) {
        self.name_font = Some(
            FontArc::try_from_vec(include_bytes!("../../../assets/fonts/EBGaramond.ttf").to_vec())
                .expect("EB Garamond parses"),
        );
        self.italic_font = Some(
            FontArc::try_from_vec(include_bytes!("../../../assets/fonts/EBGaramond-Italic.ttf").to_vec())
                .expect("EB Garamond Italic parses"),
        );
        let display =
            FontArc::try_from_vec(include_bytes!("../../../assets/fonts/Cinzel.ttf").to_vec())
                .expect("Cinzel parses");
        self.back = Some(images.add(paint_back(&display)));
    }

    /// Paints (once) and returns the card's front texture.
    pub fn front(
        &mut self,
        id: &str,
        name: &str,
        text: &str,
        cost: i32,
        images: &mut Assets<Image>,
    ) -> Handle<Image> {
        if let Some(h) = self.fronts.get(id) {
            return h.clone();
        }
        let name_font = self.name_font.clone().expect("PaintedCards::boot first");
        let italic_font = self.italic_font.clone().expect("PaintedCards::boot first");
        let emblem = self.emblem(id);
        let img = paint_front(&name_font, &italic_font, name, text, cost, emblem.as_ref());
        let handle = images.add(img);
        self.fronts.insert(id.to_string(), handle.clone());
        handle
    }

    /// Decodes the card's emblem PNG (256px gold line art).
    fn emblem(&mut self, id: &str) -> Option<image::RgbaImage> {
        if let Some(e) = self.emblems.get(id) {
            return e.clone();
        }
        let decoded = embedded_emblem(id)
            .and_then(|b| image::load_from_memory(b).ok())
            .map(|d| d.to_rgba8());
        self.emblems.insert(id.to_string(), decoded.clone());
        decoded
    }
}

/// The emblem PNG table: every shipped card's art, embedded.
fn embedded_emblem(id: &str) -> Option<&'static [u8]> {
    Some(match id {
        "agrianians" => include_bytes!("../../../assets/art/art_agrianians.png"),
        "bucephalus" => include_bytes!("../../../assets/art/art_bucephalus.png"),
        "companion_charge" => include_bytes!("../../../assets/art/art_companion_charge.png"),
        "elephant_corps" => include_bytes!("../../../assets/art/art_elephant_corps.png"),
        "greek_mercenaries" => include_bytes!("../../../assets/art/art_greek_mercenaries.png"),
        "hypaspists" => include_bytes!("../../../assets/art/art_hypaspists.png"),
        "nearchus_fleet" => include_bytes!("../../../assets/art/art_nearchus_fleet.png"),
        "nile_grain" => include_bytes!("../../../assets/art/art_nile_grain.png"),
        "parmenion_wing" => include_bytes!("../../../assets/art/art_parmenion_wing.png"),
        "persian_ransom" => include_bytes!("../../../assets/art/art_persian_ransom.png"),
        "phalanx" => include_bytes!("../../../assets/art/art_phalanx.png"),
        "roxane" => include_bytes!("../../../assets/art/art_roxane.png"),
        "royal_decree" => include_bytes!("../../../assets/art/art_royal_decree.png"),
        "royal_road" => include_bytes!("../../../assets/art/art_royal_road.png"),
        "scythian_horse" => include_bytes!("../../../assets/art/art_scythian_horse.png"),
        "siege_ladder" => include_bytes!("../../../assets/art/art_siege_ladder.png"),
        "siwa_oracle" => include_bytes!("../../../assets/art/art_siwa_oracle.png"),
        "sogdian_rock" => include_bytes!("../../../assets/art/art_sogdian_rock.png"),
        "war_chest" => include_bytes!("../../../assets/art/art_war_chest.png"),
        "wedge_formation" => include_bytes!("../../../assets/art/art_wedge_formation.png"),
        _ => return None,
    })
}

fn lerp(a: u8, b: u8, k: f32) -> u8 {
    (a as f32 + (b as f32 - a as f32) * k).round() as u8
}

fn blend_px(px: &mut [u8], over: [u8; 4], alpha: f32) {
    let a = over[3] as f32 / 255.0 * alpha;
    px[0] = lerp(px[0], over[0], a);
    px[1] = lerp(px[1], over[1], a);
    px[2] = lerp(px[2], over[2], a);
    px[3] = (a * 255.0).max(px[3] as f32) as u8;
}

fn in_rounded_off(x: u32, y: u32, x0: u32, y0: u32, x1: u32, y1: u32, r: u32) -> bool {
    if x < x0 || y < y0 || x >= x1 || y >= y1 {
        return false;
    }
    let rx = [x0 + r, x1.saturating_sub(r)];
    let ry = [y0 + r, y1.saturating_sub(r)];
    if x >= rx[0] && x < rx[1] {
        return true;
    }
    if y >= ry[0] && y < ry[1] {
        return true;
    }
    let cx = if x < rx[0] { rx[0] } else { rx[1] } as f32;
    let cy = if y < ry[0] { ry[0] } else { ry[1] } as f32;
    let (dx, dy) = (x as f32 - cx, y as f32 - cy);
    dx * dx + dy * dy <= (r as f32) * (r as f32)
}

struct Canvas {
    img: image::RgbaImage,
}

impl Canvas {
    fn new(w: u32, h: u32) -> Canvas {
        Canvas {
            img: image::RgbaImage::new(w, h),
        }
    }

    /// Vertical linear gradient inside a rounded-rect clip.
    fn gradient_rounded(&mut self, r: u32, top: [u8; 4], bottom: [u8; 4]) {
        let (w, h) = (self.img.width(), self.img.height());
        for y in 0..h {
            let k = y as f32 / h as f32;
            let base = [
                lerp(top[0], bottom[0], k),
                lerp(top[1], bottom[1], k),
                lerp(top[2], bottom[2], k),
                255,
            ];
            for x in 0..w {
                if in_rounded_off(x, y, 0, 0, w, h, r) {
                    self.img.put_pixel(x, y, image::Rgba(base));
                }
            }
        }
    }

    /// Strokes a rounded-rect border with the given thickness.
    #[allow(clippy::too_many_arguments)]
    fn stroke_rounded(
        &mut self,
        x0: u32,
        y0: u32,
        w: u32,
        h: u32,
        r: u32,
        t: u32,
        color: [u8; 4],
    ) {
        let (x1, y1) = (x0 + w, y0 + h);
        let a = color[3] as f32 / 255.0;
        for y in y0.saturating_sub(t)..(y1 + t).min(self.img.height()) {
            for x in x0.saturating_sub(t)..(x1 + t).min(self.img.width()) {
                let inside = in_rounded_off(x, y, x0, y0, x1, y1, r);
                let outer = in_rounded_off(
                    x,
                    y,
                    x0.saturating_sub(t),
                    y0.saturating_sub(t),
                    x1 + t,
                    y1 + t,
                    r + t,
                );
                if outer && !inside {
                    let px = self.img.get_pixel_mut(x, y);
                    blend_px(&mut px.0, [color[0], color[1], color[2], 255], a);
                }
            }
        }
    }

    /// Alpha-blended stroked square (meander key-pattern cell).
    fn stroke_rect(&mut self, x0: u32, y0: u32, s: u32, t: u32, color: [u8; 4]) {
        for y in y0..y0 + s {
            for x in x0..x0 + s {
                let edge = x < x0 + t || x >= x0 + s - t || y < y0 + t || y >= y0 + s - t;
                if edge {
                    let px = self.img.get_pixel_mut(x, y);
                    blend_px(&mut px.0, color, 1.0);
                }
            }
        }
    }

    /// Radial gradient fill of a rect (panel glow).
    fn radial(&mut self, x0: u32, y0: u32, w: u32, h: u32, inner: [u8; 4], outer: [u8; 4]) {
        let cx = x0 as f32 + w as f32 / 2.0;
        let cy = y0 as f32 + h as f32 * 0.15;
        let (r0, r1) = (30.0f32, 460.0f32);
        for y in y0..y0 + h {
            for x in x0..x0 + w {
                let d = ((x as f32 - cx).powi(2) + (y as f32 - cy).powi(2)).sqrt().min(r1);
                let k = ((d - r0) / (r1 - r0)).clamp(0.0, 1.0);
                self.img.put_pixel(
                    x,
                    y,
                    image::Rgba([
                        lerp(inner[0], outer[0], k),
                        lerp(inner[1], outer[1], k),
                        lerp(inner[2], outer[2], k),
                        255,
                    ]),
                );
            }
        }
    }

    /// Blits `src` scaled to `size` at (x0, y0) with alpha.
    fn blit(&mut self, src: &image::RgbaImage, x0: u32, y0: u32, size: u32, alpha: f32) {
        let scaled = image::imageops::resize(src, size, size, image::imageops::FilterType::Triangle);
        for y in 0..size {
            for x in 0..size {
                let (dx, dy) = (x0 + x, y0 + y);
                if dx >= self.img.width() || dy >= self.img.height() {
                    continue;
                }
                let s = scaled.get_pixel(x, y);
                let a = s[3] as f32 / 255.0 * alpha;
                if a <= 0.0 {
                    continue;
                }
                let px = self.img.get_pixel_mut(dx, dy);
                blend_px(&mut px.0, [s[0], s[1], s[2], 255], a);
            }
        }
    }

    /// Strokes a circle outline (cost chip).
    fn stroke_circle(&mut self, cx: u32, cy: u32, r: u32, t: u32, color: [u8; 4]) {
        let (outer, inner) = (r, r - t);
        for y in (cy - r)..(cy + r) {
            for x in (cx - r)..(cx + r) {
                let (dx, dy) = (x as f32 - cx as f32, y as f32 - cy as f32);
                let d = dx * dx + dy * dy;
                if d <= outer as f32 * outer as f32 && d >= inner as f32 * inner as f32 {
                    let px = self.img.get_pixel_mut(x, y);
                    blend_px(&mut px.0, color, 1.0);
                }
            }
        }
    }

    /// Draws one line of text centered at `cx` with its baseline at
    /// `baseline_y`; returns the line width in px.
    fn text_centered(
        &mut self,
        font: &FontArc,
        size: f32,
        text: &str,
        cx: u32,
        baseline_y: u32,
        color: [u8; 4],
    ) -> f32 {
        let scaled = font.clone().into_scaled(PxScale::from(size));
        let width = text_width(&scaled, text);
        let mut pen_x = cx as f32 - width / 2.0;
        for ch in text.chars() {
            let id = scaled.glyph_id(ch);
            let glyph =
                id.with_scale_and_position(scaled.scale, ab_glyph::point(pen_x, baseline_y as f32));
            if let Some(outlined) = font.outline_glyph(glyph) {
                let bounds = outlined.px_bounds();
                outlined.draw(|x, y, coverage| {
                    let (px, py) = (
                        (bounds.min.x + x as f32) as u32,
                        (bounds.min.y + y as f32) as u32,
                    );
                    if px >= self.img.width() || py >= self.img.height() {
                        return;
                    }
                    let a = coverage / 255.0;
                    if a <= 0.01 {
                        return;
                    }
                    let p = self.img.get_pixel_mut(px, py);
                    blend_px(&mut p.0, color, a);
                });
            }
            pen_x += scaled.h_advance(id);
        }
        width
    }
}

fn text_width(scaled: &Scaled, text: &str) -> f32 {
    let mut w = 0.0;
    let mut last: Option<ab_glyph::GlyphId> = None;
    for ch in text.chars() {
        let id = scaled.glyph_id(ch);
        w += scaled.h_advance(id);
        if let Some(prev) = last {
            w += scaled.kern(prev, id);
        }
        last = Some(id);
    }
    w
}

fn wrap_text(font: &FontArc, size: f32, text: &str, max_w: f32) -> Vec<String> {
    let scaled = font.clone().into_scaled(PxScale::from(size));
    let mut out = Vec::new();
    let mut line = String::new();
    for word in text.split(' ') {
        let test = if line.is_empty() {
            word.to_string()
        } else {
            format!("{line} {word}")
        };
        if text_width(&scaled, &test) > max_w && !line.is_empty() {
            out.push(std::mem::take(&mut line));
        }
        line = test;
    }
    if !line.is_empty() {
        out.push(line);
    }
    out
}

/// 512×716 card front: gradient plate, border + gold hairline, radial art
/// panel with the 340px emblem, shrinking name, italic flavor, cost chip.
fn paint_front(
    name_font: &FontArc,
    italic_font: &FontArc,
    name: &str,
    flavor: &str,
    cost: i32,
    emblem: Option<&image::RgbaImage>,
) -> Image {
    let mut c = Canvas::new(FACE_W, FACE_H);
    c.gradient_rounded(28, [40, 34, 25, 255], [25, 20, 16, 255]);
    c.stroke_rounded(0, 0, FACE_W, FACE_H, 28, 3, BORDER);
    c.stroke_rounded(16, 16, FACE_W - 32, FACE_H - 32, 18, 1, GOLD_SOFT);

    // Art panel: radial glow plus the emblem at 95% opacity.
    c.radial(36, 36, FACE_W - 72, 400, [36, 31, 24, 255], [18, 14, 10, 255]);
    if let Some(art) = emblem {
        c.blit(art, 86, 56, 340, 0.95);
    }

    // Name: 44px shrinking to 22 until it fits.
    let mut size = 44.0f32;
    while text_width(&name_font.clone().into_scaled(PxScale::from(size)), name) > (FACE_W - 90) as f32
        && size > 22.0
    {
        size -= 2.0;
    }
    c.text_centered(name_font, size, name, FACE_W / 2, 540, GOLD);

    // Italic flavor, wrapped and centered.
    let mut y = 596u32;
    for line in wrap_text(italic_font, 26.0, flavor, (FACE_W - 110) as f32) {
        if y > 660 {
            break;
        }
        c.text_centered(italic_font, 26.0, &line, FACE_W / 2, y, MUTED);
        y += 34;
    }

    // Cost chip: gold circle r44 + the 48px number.
    if cost > 0 {
        let (cx, cy) = (FACE_W - 78, 78u32);
        c.stroke_circle(cx, cy, 44, 4, GOLD);
        c.text_centered(name_font, 48.0, &cost.to_string(), cx, cy + 17, GOLD);
    }

    rgba_image(c.img)
}

/// 512×716 card back: darker gradient, meander key-pattern border, alpha.
fn paint_back(display: &FontArc) -> Image {
    let mut c = Canvas::new(FACE_W, FACE_H);
    c.gradient_rounded(28, [34, 28, 21, 255], [21, 16, 9, 255]);
    c.stroke_rounded(0, 0, FACE_W, FACE_H, 28, 3, BORDER);

    let step = 42u32;
    let half = step / 2;
    let mut x = 30;
    while x + half <= FACE_W - 30 {
        c.stroke_rect(x, 30, half, 3, GOLD_KEY);
        c.stroke_rect(x, FACE_H - 30 - half, half, 3, GOLD_KEY);
        x += step;
    }
    let mut y = 72;
    while y + half <= FACE_H - 72 {
        c.stroke_rect(30, y, half, 3, GOLD_KEY);
        c.stroke_rect(FACE_W - 30 - half, y, half, 3, GOLD_KEY);
        y += step;
    }

    c.text_centered(display, 300.0, "\u{0391}", FACE_W / 2, FACE_H / 2 + 105, GOLD_ALPHA);
    rgba_image(c.img)
}

/// Converts the painted buffer into a Bevy `Image` (sRGB RGBA8).
fn rgba_image(img: image::RgbaImage) -> Image {
    Image::new(
        Extent3d {
            width: img.width(),
            height: img.height(),
            depth_or_array_layers: 1,
        },
        TextureDimension::D2,
        img.into_raw(),
        TextureFormat::Rgba8UnormSrgb,
        default(),
    )
}
