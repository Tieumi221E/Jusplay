"""Prints CSS linear() approximations of springs, for web/src/tokens.css.

Parameterised like SwiftUI's spring(duration:bounce:) (WWDC23 "Animate with
springs"): duration is the perceptual duration (the period of the undamped
spring), bounce 0 is critically damped, 0.15 brisk, 0.3 bouncy. The CSS
transition length is the time the spring takes to settle within 0.1 % of its
target, printed alongside.
"""
import math

def spring(t, duration, bounce):
    w = 2 * math.pi / duration
    z = 1 - bounce  # damping ratio
    if z >= 1:
        return 1 - (1 + w * t) * math.exp(-w * t)
    wd = w * math.sqrt(1 - z * z)
    return 1 - math.exp(-z * w * t) * (math.cos(wd * t) + z * w / wd * math.sin(wd * t))

def settle(duration, bounce, eps=1e-3):
    t, last = 0.0, 0.0
    while t < 10 * duration:
        if abs(spring(t, duration, bounce) - 1) > eps:
            last = t
        t += 0.001
    return last

def linear(duration, bounce, points=32):
    T = settle(duration, bounce)
    stops = []
    for i in range(points + 1):
        t = T * i / points
        stops.append(f"{spring(t, duration, bounce):.3f}")
    stops[-1] = "1"
    return T, "linear(" + ", ".join(stops) + ")"

for name, d, b in [("smooth", 0.36, 0.0), ("snappy", 0.30, 0.15)]:
    T, css = linear(d, b)
    print(f"/* {name}: duration {d}s, bounce {b}; settles in {T*1000:.0f} ms */")
    print(f"--spring-{name}: {css};")
    print(f"--spring-{name}-t: {T*1000:.0f}ms;")
