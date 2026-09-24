"use strict";

class CircuitBreaker {
  constructor(options, now = Date.now) {
    this.windowMs = options.windowMs;
    this.minRequests = options.minRequests;
    this.failureRatio = options.failureRatio;
    this.openMs = options.openMs;
    this.halfOpenProbes = options.halfOpenProbes;
    this.now = now;

    this.state = "closed";
    this.outcomes = [];
    this.openedAt = 0;
    this.halfOpenInFlight = 0;
    this.halfOpenSuccesses = 0;
  }

  canAttempt() {
    const now = this.now();

    if (this.state === "open") {
      if (now - this.openedAt < this.openMs) {
        return false;
      }
      this.state = "half-open";
      this.halfOpenInFlight = 0;
      this.halfOpenSuccesses = 0;
    }

    if (this.state === "half-open") {
      return this.halfOpenInFlight < this.halfOpenProbes;
    }

    this.prune(now);
    return true;
  }

  acquire() {
    if (!this.canAttempt()) {
      return false;
    }
    if (this.state === "half-open") {
      this.halfOpenInFlight += 1;
    }
    return true;
  }

  record(success) {
    const now = this.now();

    if (this.state === "half-open") {
      if (success) {
        this.halfOpenInFlight = Math.max(0, this.halfOpenInFlight - 1);
        this.halfOpenSuccesses += 1;
        if (this.halfOpenSuccesses >= this.halfOpenProbes) {
          this.close();
        }
      } else {
        this.open(now);
      }
      return;
    }

    if (this.state !== "closed") {
      return;
    }

    this.outcomes.push({ at: now, failure: !success });
    this.prune(now);

    if (this.outcomes.length < this.minRequests) {
      return;
    }

    const failures = this.outcomes.reduce(
      (count, outcome) => count + (outcome.failure ? 1 : 0),
      0,
    );
    if (failures / this.outcomes.length >= this.failureRatio) {
      this.open(now);
    }
  }

  prune(now) {
    const cutoff = now - this.windowMs;
    while (this.outcomes.length > 0 && this.outcomes[0].at <= cutoff) {
      this.outcomes.shift();
    }
  }

  open(now) {
    this.state = "open";
    this.openedAt = now;
    this.halfOpenInFlight = 0;
    this.halfOpenSuccesses = 0;
  }

  close() {
    this.state = "closed";
    this.outcomes = [];
    this.halfOpenInFlight = 0;
    this.halfOpenSuccesses = 0;
  }
}

module.exports = { CircuitBreaker };
