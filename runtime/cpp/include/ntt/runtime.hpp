// ntt/runtime.hpp - clean-room C++17 mirror of the Go runtime/* layer.
//
// CLEAN-ROOM DECLARATION: this header was written from the TTCN-3
// v4.11.1 specification plus the ntt Go runtime in this repository.
// No Titan source code was consulted while writing it. The shape of
// the API mirrors Titan's surface only where the standard fixes the
// names (Verdict, Component, Port, Timer); all implementation choices
// are ntt's own.
//
// SPDX-License-Identifier: BSD-3-Clause
#pragma once

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstdint>
#include <deque>
#include <functional>
#include <memory>
#include <mutex>
#include <optional>
#include <string>
#include <unordered_map>
#include <utility>
#include <variant>
#include <vector>

namespace ntt {

// Verdict enumerates the five TTCN-3 verdicts in the precedence order
// required by clause 5.4.2 of TTCN-3 Core: None < Pass < Inconc < Fail
// < Error. The integer ordering matches the precedence so a max-merge
// can be implemented with `std::max`.
enum class Verdict : int {
    None    = 0,
    Pass    = 1,
    Inconc  = 2,
    Fail    = 3,
    Error   = 4,
};

inline const char* to_string(Verdict v) noexcept {
    switch (v) {
    case Verdict::None:   return "none";
    case Verdict::Pass:   return "pass";
    case Verdict::Inconc: return "inconc";
    case Verdict::Fail:   return "fail";
    case Verdict::Error:  return "error";
    }
    return "unknown";
}

// merge implements TTCN-3 verdict aggregation: the resulting verdict
// is the max of the two by the enum ordering above.
inline Verdict merge(Verdict a, Verdict b) noexcept {
    return static_cast<Verdict>(std::max(static_cast<int>(a), static_cast<int>(b)));
}

// Context is the per-execution environment the codegen threads
// through every generated function. The fields are intentionally
// minimal in M8; M9 will grow logging, parameter bag, and module-
// configuration entries onto it.
struct Context {
    Verdict local_verdict { Verdict::None };
    std::string testcase_name;
};

// Timer mirrors runtime/timer.Timer in Go. The state machine and
// public API surface are identical so the same IR ops drive both
// backends.
class Timer {
public:
    enum class State { Idle, Running, Expired };

    explicit Timer(std::string name = {}) : name_(std::move(name)) {}

    void start(std::chrono::duration<double> d) {
        std::lock_guard<std::mutex> lk(mu_);
        // Cast the user-supplied double-precision duration to the
        // steady_clock's native rep (typically nanoseconds) before
        // arithmetic - otherwise `now() + d` produces a time_point
        // with a double-based duration that won't assign to deadline_.
        auto nd = std::chrono::duration_cast<std::chrono::steady_clock::duration>(d);
        deadline_ = std::chrono::steady_clock::now() + nd;
        duration_ = d;
        state_    = State::Running;
    }

    void stop() {
        std::lock_guard<std::mutex> lk(mu_);
        state_ = State::Idle;
    }

    double read() const {
        std::lock_guard<std::mutex> lk(mu_);
        if (state_ != State::Running) return 0.0;
        auto elapsed = std::chrono::steady_clock::now() - (deadline_ - duration_);
        return std::chrono::duration<double>(elapsed).count();
    }

    bool running() const {
        std::lock_guard<std::mutex> lk(mu_);
        return state_ == State::Running;
    }

    bool timeout() {
        std::lock_guard<std::mutex> lk(mu_);
        if (state_ != State::Running) return false;
        if (std::chrono::steady_clock::now() < deadline_) return false;
        state_ = State::Expired;
        return true;
    }

    State state() const {
        std::lock_guard<std::mutex> lk(mu_);
        return state_;
    }

    const std::string& name() const noexcept { return name_; }

private:
    mutable std::mutex mu_;
    std::string name_;
    State state_ { State::Idle };
    std::chrono::steady_clock::time_point deadline_ {};
    std::chrono::duration<double> duration_ {};
};

// Envelope carries a payload between Ports. The payload variant lets
// codecs hand off either a typed value (handled via decode) or a raw
// byte buffer (handled by transport-only ports).
struct Envelope {
    std::string sender;
    std::vector<std::uint8_t> bytes;
    std::string from_port;
};

// Port mirrors runtime/port.Port in Go: an unbounded message queue
// with non-destructive Peek, destructive Try, and a Wake notification
// used by the alt scheduler.
class Port {
public:
    enum class Kind { Message, Procedure, Mixed };

    Port(std::string name, Kind k = Kind::Message)
        : name_(std::move(name)), kind_(k) {}

    void send(Envelope e) {
        {
            std::lock_guard<std::mutex> lk(mu_);
            queue_.push_back(std::move(e));
        }
        cv_.notify_all();
    }

    std::optional<Envelope> try_recv() {
        std::lock_guard<std::mutex> lk(mu_);
        if (queue_.empty()) return std::nullopt;
        auto e = std::move(queue_.front());
        queue_.pop_front();
        return e;
    }

    const Envelope* peek() const {
        std::lock_guard<std::mutex> lk(mu_);
        return queue_.empty() ? nullptr : &queue_.front();
    }

    void drop_head() {
        std::lock_guard<std::mutex> lk(mu_);
        if (!queue_.empty()) queue_.pop_front();
    }

    const std::string& name() const noexcept { return name_; }
    Kind kind() const noexcept { return kind_; }

private:
    mutable std::mutex mu_;
    std::condition_variable cv_;
    std::deque<Envelope> queue_;
    std::string name_;
    Kind kind_;
};

// Component mirrors runtime/component.Component. The component
// registry is omitted from the header-only build to keep the
// dependency surface minimal; backend/cpp emits explicit ctor calls
// when it needs new components.
class Component {
public:
    enum class Role  { MTC, PTC, System };
    enum class State { Initial, Running, Done, Killed };

    Component(std::int64_t id, std::string name, Role role)
        : id_(id), name_(std::move(name)), role_(role) {}

    std::int64_t id() const noexcept { return id_; }
    const std::string& name() const noexcept { return name_; }
    Role role() const noexcept { return role_; }
    State state() const noexcept { return state_.load(); }

    void mark_done()   { state_.store(State::Done); }
    void mark_killed() { state_.store(State::Killed); }

private:
    std::int64_t id_;
    std::string name_;
    Role role_;
    std::atomic<State> state_ { State::Initial };
};

// TestPort is the C++ counterpart of runtime/port/api.TestPort. It's
// the integration surface for telco test ports written in C++: a port
// implementer subclasses TestPort and is wired into the dispatch
// table generated by backend/cpp.
class TestPort {
public:
    virtual ~TestPort() = default;
    virtual void on_map() {}
    virtual void on_unmap() {}
    virtual void on_send(const Envelope&) {}
};

// Value is the union of TTCN-3 base values reachable from the IR's
// constant opcodes. The codegen never produces values it cannot type,
// so the variant alternatives match ir.Type exactly.
using Value = std::variant<
    std::monostate,
    std::int64_t,
    double,
    bool,
    std::string,
    std::vector<std::uint8_t>
>;

// TestcaseExec is the C++ mirror of runtime.TestcaseExec - the per-
// testcase execution context that the codegen passes through every
// generated function. setverdict / getverdict / log all funnel
// through this object so the harness can collect verdicts across a
// whole suite. Thread-safe: the snapshot scheduler that drives
// `alt` runs goroutines (in Go) or std::async tasks (here) in
// parallel and they all share one exec.
class TestcaseExec {
public:
    explicit TestcaseExec(std::string name) : name_(std::move(name)) {}

    void setVerdict(Verdict v, std::string reason = {}) {
        std::lock_guard<std::mutex> lk(mu_);
        if (static_cast<int>(v) > static_cast<int>(verdict_)) {
            verdict_ = v;
            if (!reason.empty() && (v == Verdict::Fail || v == Verdict::Error)) {
                reason_ = std::move(reason);
            }
        }
    }

    Verdict getVerdict() const {
        std::lock_guard<std::mutex> lk(mu_);
        return verdict_;
    }

    const std::string& reason() const noexcept { return reason_; }

    void log(std::string line) {
        std::lock_guard<std::mutex> lk(mu_);
        log_.push_back(std::move(line));
    }

    const std::vector<std::string>& logs() const noexcept { return log_; }

    const std::string& name() const noexcept { return name_; }

private:
    mutable std::mutex mu_;
    std::string name_;
    Verdict verdict_ { Verdict::None };
    std::string reason_;
    std::vector<std::string> log_;
};

// SuiteResult aggregates the verdicts of every testcase a binary
// runs. It is what the CLI harness (`ntt-cpp-runner`) writes out as
// JSON / TAP / JUnit. Verdict aggregation across testcases is the
// max of the per-testcase verdicts, mirroring runtime/report on the
// Go side.
struct SuiteResult {
    std::string suite_name;
    std::vector<std::pair<std::string, Verdict>> cases;

    Verdict overall() const {
        Verdict v = Verdict::None;
        for (const auto& c : cases) {
            v = merge(v, c.second);
        }
        return v;
    }
};

// Run drives one testcase: it constructs a TestcaseExec, invokes the
// generated entry point, and folds the result into a SuiteResult.
// The Fn signature matches what backend/cpp emits for void-returning
// testcases: `Verdict tc(Context& ctx)`. Helper functions stay outside
// this overload.
template <typename Fn>
inline Verdict Run(SuiteResult& suite, const std::string& name, Fn&& fn) {
    Context ctx;
    ctx.testcase_name = name;
    Verdict v = fn(ctx);
    suite.cases.emplace_back(name, v);
    return v;
}

} // namespace ntt
