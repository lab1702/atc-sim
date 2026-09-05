import * as THREE from "./vendor/three.module.js";

const rad = Math.PI / 180;
const point3 = (p, height = 0) => new THREE.Vector3(p.x, height, -p.y);

// Airport pavement and buildings are batched into a few GPU buffers. Each view
// uses a different camera and palette, but the same metre-based airport geometry.
class Batch {
  positions = [];
  colors = [];
  triangle(a, b, c, color) {
    const col = new THREE.Color(color);
    for (const p of [a, b, c]) {
      this.positions.push(...p);
      this.colors.push(col.r, col.g, col.b);
    }
  }
  quad(a, b, c, d, color) {
    this.triangle(a, b, c, color);
    this.triangle(a, c, d, color);
  }
  ribbon(a, b, width, height, color) {
    const dx = b.x - a.x,
      dy = b.y - a.y,
      len = Math.hypot(dx, dy);
    if (len < 0.01) return;
    const ox = ((-dy / len) * width) / 2,
      oy = ((dx / len) * width) / 2;
    this.quad(
      [a.x + ox, height, -a.y - oy],
      [b.x + ox, height, -b.y - oy],
      [b.x - ox, height, -b.y + oy],
      [a.x - ox, height, -a.y + oy],
      color,
    );
  }
  polygon(points, height, color, walls = false) {
    if (points.length < 3) return;
    const ps = points.slice();
    if (ps[0].x === ps.at(-1).x && ps[0].y === ps.at(-1).y) ps.pop();
    for (const [a, b, c] of THREE.ShapeUtils.triangulateShape(
      ps.map((p) => new THREE.Vector2(p.x, p.y)),
      [],
    ))
      this.triangle(
        [ps[a].x, height, -ps[a].y],
        [ps[b].x, height, -ps[b].y],
        [ps[c].x, height, -ps[c].y],
        color,
      );
    if (walls)
      for (let i = 0; i < ps.length; i++) {
        const a = ps[i],
          b = ps[(i + 1) % ps.length];
        this.quad(
          [a.x, 0, -a.y],
          [b.x, 0, -b.y],
          [b.x, height, -b.y],
          [a.x, height, -a.y],
          new THREE.Color(color).multiplyScalar(0.65),
        );
      }
  }
  mesh(lit = false) {
    const g = new THREE.BufferGeometry();
    g.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(this.positions, 3),
    );
    g.setAttribute("color", new THREE.Float32BufferAttribute(this.colors, 3));
    g.computeVertexNormals();
    return new THREE.Mesh(
      g,
      lit
        ? new THREE.MeshLambertMaterial({
            vertexColors: true,
            side: THREE.DoubleSide,
          })
        : new THREE.MeshBasicMaterial({
            vertexColors: true,
            side: THREE.DoubleSide,
          }),
    );
  }
}

function environment(airport, mode) {
  const map = mode === "map",
    scene = new THREE.Scene();
  scene.background = new THREE.Color(map ? "#112126" : "#94b3c4");
  if (!map) {
    scene.fog = new THREE.Fog("#94b3c4", 9500, 28000);
    scene.add(new THREE.HemisphereLight("#e1eff7", "#5b675c", 2.2));
    const sun = new THREE.DirectionalLight("#fff1d4", 2.1);
    sun.position.set(-5000, 8000, 2000);
    scene.add(sun);
  }
  const ground = new Batch();
  ground.quad(
    [-35000, -2, -35000],
    [35000, -2, -35000],
    [35000, -2, 35000],
    [-35000, -2, 35000],
    map ? "#112126" : "#60766c",
  );
  scene.add(ground.mesh());
  const b = new Batch();
  for (const apron of airport.aprons || [])
    b.polygon(apron.points, 0.05, map ? "#203139" : "#818d8c");
  for (const t of airport.taxiways)
    for (let i = 1; i < t.points.length; i++) {
      b.ribbon(
        t.points[i - 1],
        t.points[i],
        t.width || 23,
        0.12,
        map ? "#354b51" : "#929b99",
      );
      b.ribbon(
        t.points[i - 1],
        t.points[i],
        map ? 1.2 : 0.5,
        0.17,
        map ? "#647b73" : "#d8c17c",
      );
    }
  for (const r of airport.runways) {
    b.ribbon(r.start, r.end, r.width + 8, 0.22, map ? "#718483" : "#c4c8c0");
    b.ribbon(r.start, r.end, r.width, 0.25, map ? "#24393f" : "#4b565b");
    const dx = r.end.x - r.start.x,
      dy = r.end.y - r.start.y,
      l = Math.hypot(dx, dy),
      ux = dx / l,
      uy = dy / l;
    for (let t = 65; t < l - 70; t += 60)
      b.ribbon(
        { x: r.start.x + ux * t, y: r.start.y + uy * t },
        { x: r.start.x + ux * (t + 30), y: r.start.y + uy * (t + 30) },
        map ? 3 : 1,
        0.4,
        map ? "#b5c8c3" : "#e7e9de",
      );
    for (const [p, sign] of [
      [r.start, 1],
      [r.end, -1],
    ])
      for (let n = -3; n <= 3; n++) {
        if (n === 0) continue;
        const a = {
          x: p.x + ux * sign * 20 - (uy * n * r.width) / 9,
          y: p.y + uy * sign * 20 + (ux * n * r.width) / 9,
        };
        b.ribbon(
          a,
          { x: a.x + ux * sign * 35, y: a.y + uy * sign * 35 },
          3,
          0.4,
          map ? "#c0cfca" : "#e5e9dd",
        );
      }
    if (map)
      for (let n = 1; n <= 12; n++) {
        const a = { x: r.start.x - ux * n * 240, y: r.start.y - uy * n * 240 };
        b.ribbon(a, { x: a.x - ux * 90, y: a.y - uy * 90 }, 2, -0.2, "#2b484f");
      }
  }
  for (const building of airport.buildings)
    b.polygon(
      building.points,
      map ? 0.8 : building.height || 18,
      map ? "#3b535e" : "#a8b8bc",
      !map,
    );
  for (const gate of airport.gates) {
    b.ribbon(
      { x: gate.x - 6, y: gate.y },
      { x: gate.x + 6, y: gate.y },
      12,
      0.6,
      map ? "#88a299" : "#d7d2aa",
    );
  }
  scene.add(b.mesh(!map));
  if (map) {
    const grid = [];
    for (let i = -12000; i <= 12000; i += 500) {
      grid.push(
        i,
        -0.5,
        -12000,
        i,
        -0.5,
        12000,
        -12000,
        -0.5,
        i,
        12000,
        -0.5,
        i,
      );
    }
    const g = new THREE.BufferGeometry();
    g.setAttribute("position", new THREE.Float32BufferAttribute(grid, 3));
    scene.add(
      new THREE.LineSegments(
        g,
        new THREE.LineBasicMaterial({
          color: "#1c333a",
          transparent: true,
          opacity: 0.55,
        }),
      ),
    );
  }
  return scene;
}

function airplaneGeometry() {
  const b = new Batch(),
    c = "#ffffff";
  b.polygon(
    [
      { x: 0, y: 20 },
      { x: -3, y: 12 },
      { x: -3, y: -15 },
      { x: 0, y: -21 },
      { x: 3, y: -15 },
      { x: 3, y: 12 },
    ],
    2,
    c,
    true,
  );
  b.polygon(
    [
      { x: 0, y: 7 },
      { x: -19, y: -3 },
      { x: -19, y: -7 },
      { x: 0, y: -2 },
      { x: 19, y: -7 },
      { x: 19, y: -3 },
    ],
    2,
    c,
  );
  b.polygon(
    [
      { x: 0, y: -11 },
      { x: -8, y: -18 },
      { x: -8, y: -20 },
      { x: 0, y: -17 },
      { x: 8, y: -20 },
      { x: 8, y: -18 },
    ],
    3,
    c,
  );
  b.triangle([0, 2, 12], [0, 10, 19], [0, 2, 20], c);
  const mesh = b.mesh();
  mesh.geometry.deleteAttribute("color");
  mesh.material.dispose();
  return mesh.geometry;
}
const aircraftGeometry = airplaneGeometry();

export class AirportView {
  constructor(container, labels, airport, mode, onSelect) {
    this.container = container;
    this.labels = labels;
    this.airport = airport;
    this.mode = mode;
    this.onSelect = onSelect;
    this.aircraft = new Map();
    this.staticLabels = [];
    this.selected = null;
    this.renderer = new THREE.WebGLRenderer({
      antialias: true,
      alpha: false,
      powerPreference: "high-performance",
    });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    this.renderer.setClearColor("#112126");
    this.renderer.domElement.setAttribute(
      "aria-label",
      mode === "map"
        ? "Interactive DTW ground radar"
        : "Interactive DTW 3D tower view",
    );
    container.prepend(this.renderer.domElement);
    this.scenes = {
      map: environment(airport, "map"),
      tower: environment(airport, "tower"),
    };
    this.mapCamera = new THREE.OrthographicCamera(
      -5000,
      5000,
      5000,
      -5000,
      1,
      50000,
    );
    this.mapCamera.up.set(0, 0, -1);
    this.towerCamera = new THREE.PerspectiveCamera(65, 1, 1, 50000);
    this.center = {
      x: (airport.bounds.minX + airport.bounds.maxX) / 2,
      y: (airport.bounds.minY + airport.bounds.maxY) / 2,
    };
    this.span = 6200;
    this.hasRendered = false;
    this.yaw = 37 * rad;
    this.pitch = -0.04;
    this.fov = 65;
    this.tracking = false;
    this.routes = {};
    for (const m of ["map", "tower"]) {
      const route = new THREE.Line(
        new THREE.BufferGeometry(),
        new THREE.LineDashedMaterial({
          color: "#c4f1dd",
          dashSize: 18,
          gapSize: 10,
          transparent: true,
          opacity: 0.8,
          depthTest: false,
        }),
      );
      this.scenes[m].add(route);
      this.routes[m] = route;
    }
    this.makeStaticLabels();
    this.bindControls();
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(container);
    this.resize();
    this.reset();
  }
  makeStaticLabels() {
    const add = (text, p, kind) => {
      const el = document.createElement("span");
      el.className = `airport-label ${kind}`;
      el.textContent = text;
      this.labels.append(el);
      this.staticLabels.push({ el, p, kind });
    };
    for (const r of this.airport.runways) {
      const dx = r.end.x - r.start.x,
        dy = r.end.y - r.start.y,
        len = Math.hypot(dx, dy);
      add(
        r.id,
        { x: r.start.x - (dx / len) * 100, y: r.start.y - (dy / len) * 100 },
        "runway",
      );
      add(
        r.opposite,
        { x: r.end.x + (dx / len) * 100, y: r.end.y + (dy / len) * 100 },
        "runway",
      );
    }
    const named = new Map();
    for (const t of this.airport.taxiways) {
      if (!t.name || t.name.length > 3 || t.width < 15) continue;
      const length = t.points.reduce(
        (sum, p, i) =>
          sum +
          (i
            ? Math.hypot(p.x - t.points[i - 1].x, p.y - t.points[i - 1].y)
            : 0),
        0,
      );
      if (!named.has(t.name) || named.get(t.name).length < length)
        named.set(t.name, { length, t });
    }
    for (const { t } of named.values())
      add(
        t.name,
        t.points[Math.floor(t.points.length / 2)],
        /^[A-Z]{1,2}$/.test(t.name) ? "taxiway" : "taxiway-detail",
      );
    for (const b of this.airport.buildings.filter((b) =>
      /^[AD] Concourse/i.test(b.name),
    )) {
      const p = b.points.reduce(
        (v, p) => ({
          x: v.x + p.x / b.points.length,
          y: v.y + p.y / b.points.length,
        }),
        { x: 0, y: 0 },
      );
      add(b.name.toUpperCase(), p, "terminal");
    }
    add("TWR", this.airport.tower, "tower");
  }
  bindControls() {
    const el = this.renderer.domElement;
    let drag = null;
    el.addEventListener("pointerdown", (e) => {
      if (e.button !== 0) return;
      drag = {
        x: e.clientX,
        y: e.clientY,
        startX: e.clientX,
        startY: e.clientY,
        moved: false,
      };
      el.setPointerCapture(e.pointerId);
    });
    el.addEventListener("pointermove", (e) => {
      if (!drag) return;
      const dx = e.clientX - drag.x,
        dy = e.clientY - drag.y;
      drag.moved ||=
        Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) > 5;
      drag.x = e.clientX;
      drag.y = e.clientY;
      if (this.mode === "map") {
        this.center.x -= (dx * this.span) / this.height;
        this.center.y += (dy * this.span) / this.height;
      } else {
        this.tracking = false;
        this.yaw -= dx * 0.004;
        this.pitch = THREE.MathUtils.clamp(this.pitch + dy * 0.004, -1.15, 0.7);
      }
    });
    const end = (e) => {
      if (drag && !drag.moved) {
        const rect = el.getBoundingClientRect();
        let nearest = null,
          best = 24;
        for (const [id, obj] of this.aircraft) {
          const p = this.project(obj[this.mode].position);
          if (!p.visible) continue;
          const dist = Math.hypot(
            p.x - e.clientX + rect.left,
            p.y - e.clientY + rect.top,
          );
          if (dist < best) {
            nearest = id;
            best = dist;
          }
        }
        if (nearest) this.onSelect(nearest);
      }
      drag = null;
    };
    el.addEventListener("pointerup", end);
    el.addEventListener("pointercancel", () => (drag = null));
    el.addEventListener(
      "wheel",
      (e) => {
        e.preventDefault();
        this.zoom(Math.exp(e.deltaY * 0.001));
      },
      { passive: false },
    );
    el.addEventListener("webglcontextlost", (e) => {
      e.preventDefault();
      this.container.dataset.renderError =
        "Graphics context lost. Reload to reconnect.";
    });
  }
  resize() {
    this.width = this.container.clientWidth;
    this.height = this.container.clientHeight;
    if (this.width && this.height)
      this.renderer.setSize(this.width, this.height, false);
  }
  setMode(mode) {
    this.mode = mode;
    this.renderer.domElement.setAttribute(
      "aria-label",
      mode === "map"
        ? "Interactive DTW ground radar"
        : "Interactive DTW 3D tower view",
    );
  }
  zoom(factor) {
    if (this.mode === "map")
      this.span = THREE.MathUtils.clamp(this.span * factor, 1000, 28000);
    else this.fov = THREE.MathUtils.clamp(this.fov * factor, 12, 90);
  }
  reset() {
    const b = this.airport.bounds;
    this.center = { x: (b.minX + b.maxX) / 2, y: (b.minY + b.maxY) / 2 };
    this.span =
      Math.max(
        b.maxY - b.minY,
        (b.maxX - b.minX) / Math.max(0.3, this.width / this.height),
      ) * 1.2;
    this.hasRendered = false;
    this.yaw = 37 * rad;
    this.pitch = -0.04;
    this.fov = 65;
    this.tracking = false;
  }
  select(id) {
    this.selected = id;
    const obj = this.aircraft.get(id);
    if (obj && this.mode === "map" && this.hasRendered) {
      const p = this.project(obj.map.position);
      if (
        !p.visible ||
        p.x < 30 ||
        p.x > this.width - 110 ||
        p.y < 30 ||
        p.y > this.height - 50
      ) {
        const b = this.airport.bounds,
          q = obj.data.position,
          minX = Math.min(b.minX, q.x),
          maxX = Math.max(b.maxX, q.x),
          minY = Math.min(b.minY, q.y),
          maxY = Math.max(b.maxY, q.y);
        this.center = { x: (minX + maxX) / 2, y: (minY + maxY) / 2 };
        this.span =
          Math.max(maxY - minY, (maxX - minX) / (this.width / this.height)) *
          1.2;
      }
    }
  }
  track() {
    if (this.selected) {
      this.tracking = !this.tracking;
      if (this.tracking) this.fov = 40;
    }
  }
  update(state) {
    const seen = new Set();
    for (const a of state.aircraft) {
      if (a.phase === "complete") continue;
      seen.add(a.id);
      let obj = this.aircraft.get(a.id);
      if (!obj) {
        obj = {};
        for (const m of ["map", "tower"]) {
          const material =
            m === "map"
              ? new THREE.MeshBasicMaterial({
                  color: "#8ee0ca",
                  depthTest: false,
                })
              : new THREE.MeshLambertMaterial({ color: "#e7ebdf" });
          obj[m] = new THREE.Mesh(aircraftGeometry, material);
          obj[m].renderOrder = 5;
          this.scenes[m].add(obj[m]);
        }
        obj.el = document.createElement("button");
        obj.el.className = "aircraft-label";
        obj.el.addEventListener("click", () => this.onSelect(a.id));
        this.labels.append(obj.el);
        obj.previous = a;
        obj.current = a;
        obj.received = performance.now();
        this.aircraft.set(a.id, obj);
      }
      obj.previous = obj.current;
      obj.current = a;
      obj.data = a;
      obj.received = performance.now();
      const selected = a.id === this.selected;
      obj.map.material.color.set(
        selected ? "#ffffff" : a.kind === "arrival" ? "#edbd78" : "#8ee0ca",
      );
      obj.tower.material.color.set(
        selected ? "#c2fff0" : a.kind === "arrival" ? "#f2d29d" : "#dcebe8",
      );
      obj.el.className = `aircraft-label ${a.kind}${selected ? " selected" : ""}`;
      const text = `${a.callsign}<span>${Math.round(a.altitude / 100)
        .toString()
        .padStart(3, "0")} · ${Math.round(a.speed)} kt</span>`;
      if (obj.el.innerHTML !== text) obj.el.innerHTML = text;
      obj.el.setAttribute("aria-label", `Select ${a.callsign}`);
    }
    for (const [id, obj] of this.aircraft)
      if (!seen.has(id)) {
        for (const m of ["map", "tower"]) {
          this.scenes[m].remove(obj[m]);
          obj[m].material.dispose();
        }
        obj.el.remove();
        this.aircraft.delete(id);
      }
    const a = state.aircraft.find((a) => a.id === this.selected);
    const key = JSON.stringify(a?.route || []);
    if (key !== this.routeKey) {
      this.routeKey = key;
      for (const m of ["map", "tower"]) {
        const line = this.routes[m];
        line.geometry.dispose();
        line.geometry = new THREE.BufferGeometry().setFromPoints(
          (a?.route || []).map((p) => point3(p, m === "map" ? 8 : 1)),
        );
        line.computeLineDistances();
      }
    }
  }
  project(vector) {
    const p = vector
      .clone()
      .project(this.mode === "map" ? this.mapCamera : this.towerCamera);
    return {
      x: ((p.x + 1) * this.width) / 2,
      y: ((1 - p.y) * this.height) / 2,
      visible:
        p.z > -1 && p.z < 1 && Math.abs(p.x) < 1.02 && Math.abs(p.y) < 1.02,
    };
  }
  render(now) {
    if (!this.width || !this.height) return;
    const ratio = this.width / this.height;
    this.mapCamera.left = (-this.span * ratio) / 2;
    this.mapCamera.right = (this.span * ratio) / 2;
    this.mapCamera.top = this.span / 2;
    this.mapCamera.bottom = -this.span / 2;
    this.mapCamera.position.set(this.center.x, 20000, -this.center.y);
    this.mapCamera.lookAt(this.center.x, 0, -this.center.y);
    this.mapCamera.updateProjectionMatrix();
    this.mapCamera.updateMatrixWorld();
    for (const [id, obj] of this.aircraft) {
      const a = obj.current,
        b = obj.previous,
        t = THREE.MathUtils.clamp((now - obj.received) / 100, 0, 1),
        x = THREE.MathUtils.lerp(b.position.x, a.position.x, t),
        y = THREE.MathUtils.lerp(b.position.y, a.position.y, t),
        h = THREE.MathUtils.lerp(b.altitude, a.altitude, t);
      let dh = ((a.heading - b.heading + 540) % 360) - 180;
      const heading = (b.heading + dh * t) * rad;
      obj.map.position.set(x, 10, -y);
      obj.map.rotation.y = -heading;
      obj.map.scale.setScalar(
        Math.max(0.6, this.span / Math.max(this.height, 1) / 3.2),
      );
      obj.tower.position.set(
        x,
        Math.max(1, (h - this.airport.elevation) * 0.3048),
        -y,
      );
      obj.tower.rotation.y = -heading;
      obj.el.classList.toggle("selected", id === this.selected);
    }
    const tower = this.airport.tower;
    this.towerCamera.position.set(tower.x, tower.height, -tower.y);
    this.towerCamera.fov = this.fov;
    this.towerCamera.aspect = ratio;
    const tracked = this.tracking && this.aircraft.get(this.selected);
    if (tracked) this.towerCamera.lookAt(tracked.tower.position);
    else
      this.towerCamera.lookAt(
        tower.x + Math.sin(this.yaw) * 1000,
        tower.height + Math.sin(this.pitch) * 1000,
        -tower.y - Math.cos(this.yaw) * 1000,
      );
    this.towerCamera.updateProjectionMatrix();
    this.towerCamera.updateMatrixWorld();
    const camera = this.mode === "map" ? this.mapCamera : this.towerCamera;
    this.renderer.render(this.scenes[this.mode], camera);
    this.hasRendered = true;
    const placed = [];
    for (const obj of [...this.aircraft.values()].sort(
      (a, b) => (b.data.id === this.selected) - (a.data.id === this.selected),
    )) {
      const p = this.project(obj[this.mode].position);
      obj.el.style.display = "none";
      if (!p.visible) continue;
      const x = THREE.MathUtils.clamp(p.x, 0, this.width - 110),
        base = THREE.MathUtils.clamp(p.y, 44, this.height - 18);
      const candidates = [0, 44, -44, 88, -88, 132, -132]
        .map((d) => base + d)
        .filter((y) => y >= 44 && y <= this.height - 18);
      const y = candidates.find(
        (y) =>
          !placed.some(
            (q) => Math.abs(q.x - x) < 105 && Math.abs(q.y - y) < 42,
          ),
      );
      if (y === undefined) continue;
      placed.push({ x, y });
      obj.el.style.display = "";
      obj.el.style.left = `${x}px`;
      obj.el.style.top = `${y}px`;
    }
    const staticPlaced = [];
    for (const item of this.staticLabels) {
      const p = this.project(point3(item.p, 0.6));
      let visible = p.visible && this.mode === "map";
      if (item.kind === "taxiway") visible &&= this.span < 7500;
      if (item.kind === "taxiway-detail") visible &&= this.span < 3200;
      if (visible && item.kind.startsWith("taxiway")) {
        visible = !staticPlaced.some(
          (q) => Math.abs(q.x - p.x) < 30 && Math.abs(q.y - p.y) < 17,
        );
        if (visible) staticPlaced.push(p);
      }
      item.el.style.display = visible ? "" : "none";
      if (visible) {
        item.el.style.left = `${p.x}px`;
        item.el.style.top = `${p.y}px`;
      }
    }
  }
}
