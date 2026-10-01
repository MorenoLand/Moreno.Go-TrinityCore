-- Battle for Mount Hyjal trash scripts — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/hyjal_trash.cpp
-- (1518 lines; npc_giant_infernal / npc_abomination / npc_ghoul /
-- npc_necromancer / npc_banshee / npc_crypt_fiend / npc_fel_stalker /
-- npc_frost_wyrm / npc_gargoyle : hyjal_trashAI : public EscortAI;
-- alliance_rifleman : ScriptedAI; AddSC_hyjal_trash at end registers all 10;
-- kalimdor loader decl 29 / call 142 per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms hyjal_trash.cpp as the sole
-- source of all ten script names (loader lines only otherwise).
-- Entry: hyjal.h HYCreaturesIds names all nine trash mobs under the
-- "Trash Mobs summoned in waves" comment — GHOUL = 17895 / CRYPT_FIEND =
-- 17897 / ABOMINATION = 17898 / NECROMANCER = 17899 / BANSHEE = 17905 /
-- GARGOYLE = 17906 / FROST_WYRM = 17907 / GIANT_INFERNAL = 17908 /
-- FEL_STALKER = 17916 — and hyjal_trash.cpp's own overrun UpdateAI switches
-- on GARGOYLE / ABOMINATION / GHOUL. A header enum naming the entries is
-- stronger than the kalecgos bar the jeklik (14517) and aku_mai (4829)
-- registrations already met; the creature_template ScriptName binding
-- stays DB-side. instance_hyjal.cpp cases none of these entries in
-- OnCreatureCreate (they are wave-spawned, not GUID-bound).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat spell legs; the escort/wave
-- machine needs an EscortAI + instance bridge and has none — see below):
-- - npc_giant_infernal: Immolation 37059 self-cast once on the first combat
--   tick (C++ imol flag, non-triggered); Flame Buffet 31724 on victim init
--   2s -> repeat 7s, triggered in C++ (DoCastVictim(..., true)).
-- - npc_abomination: Disease Cloud 31607 self-cast refresh — C++ re-casts
--   whenever the aura is missing (no timer; modeled as a 1s poll, jeklik
--   convention), non-triggered; Knockdown 31610 on victim init 10s ->
--   repeat {15s,25s}, non-triggered.
-- - npc_ghoul: Frenzy 31540 self-cast init {5s,10s} -> repeat {15s,30s},
--   non-triggered.
-- - npc_necromancer: Shadow Bolt 31627 on victim init {1s,6s} ->
--   repeat {20s,30s}, non-triggered.
-- - npc_banshee: Banshee Curse 31651 on victim init {20s,25s} -> same repeat;
--   Banshee Wail 38183 on victim init {15s,20s} -> same repeat; Anti-Magic
--   Shell 31662 self init {50s,60s} -> same repeat; all non-triggered.
-- - npc_crypt_fiend: Web 28991 on victim init {20s,25s} -> same repeat,
--   non-triggered.
-- - npc_fel_stalker: Mana Burn 31729 on victim init {9s,14s} -> same
--   repeat, non-triggered.
-- - npc_frost_wyrm: Frost Breath 31688 init 5s; C++ casts only when the
--   victim is beyond 25 (then StopMoving/Clear and re-arm 4s); when the
--   victim is within 25 the timer stays expired and retries next tick —
--   modeled as a 1s retry (batrider bomb precedent). Non-triggered.
-- - npc_gargoyle: Gargoyle Strike 31664 init {2s,7s}; C++ casts only when
--   the victim is within 20 (StopMoving/Clear, re-arm {2s,3s}) and otherwise
--   MovePoints toward it with StrikeTimer = 0 — no movement bridge, so the
--   out-of-range leg is modeled as a 1s retry. Non-triggered.
-- Unmodeled (documented-only, no bridges):
-- - The entire hyjal_trashAI escort machine: IsEvent/IsOverrun wave
--   waypoints (HordeWPs / AllianceWPs / FrostWyrmWPs / GargoyleWPs /
--   FlyPathWPs / AllianceOverrunWP[55] / HordeOverrunWP[21] tables),
--   WaypointReached AddThreat legs (instance GUID lookups of DATA_THRALL /
--   DATA_JAINAPROUDMOORE), DespawnOrUnsummon / SetDespawnAtEnd arms, the
--   IsOverrun DummyTarget gargoyle strike leg — EscortAI movement plus
--   instance-script model, both blocked (standing).
-- - hyjal_trashAI::DamageTaken: instance->SetData(DATA_RAIDDAMAGE, damage)
--   raid-damage accounting (instance-data bridge blocked); ::JustDied:
--   instance->SetData(DATA_TRASH, 0) wave signal (blocked) and
--   RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE) below the
--   MINRAIDDAMAGE (700000) / world-boss damageTaken gate — no flag bridge
--   (jeklik NOT_SELECTABLE precedent).
-- - npc_giant_infernal: the meteor spawn choreography (MODEL_INVIS 11686,
--   UNIT_FLAG_NOT_SELECTABLE / NON_ATTACKABLE flags, Delay, world-trigger
--   21987 summon + SPELL_METEOR 33814 visual) — flags, model swap, and
--   summon have no bridges (maiden/jeklik precedent); the IsEvent waypoint
--   leg (HordeWPs[7] in front of Thrall) — no escort bridge.
-- - npc_frost_wyrm: Reset SetDisableGravity(true), the chase/StopMoving/
--   Clear movement legs, JustDied ground-position despawn visuals —
--   no bridges.
-- - npc_gargoyle: Reset SetDisableGravity(true), the Zpos-descending
--   MovePoint approach, forcemove random-target AttackStart, JustDied
--   ground-position visuals — no bridges.
-- - npc_necromancer: KilledUnit skeleton spawns 17902/17903 at +/-3
--   offsets TEMPSUMMON_TIMED_DESPAWN 60s + SummonList bookkeeping —
--   no summon model (jeklik precedent). Event 3 not registered.
-- - npc_banshee / crypt fiend / fel stalker / abomination / ghoul:
--   IsEvent escort start legs (Start(false/true, true) with
--   DATA_ALLIANCE_RETREAT-dependent HordeWPs/AllianceWPs) — no bridge.
-- npc_alliance_rifleman: NOT registered — zero C++ entry evidence (no
-- hyjal.h constant, zero "alliance_rifleman" hits outside hyjal_trash.cpp
-- and the loader): registering would invent an identifier (gelihast
-- precedent). Its combat arms (MoveInLineOfSight attack within 30,
-- Exploding Shot 7896 with SPELLVALUE_BASE_POINT0 500 + rand%700, evade
-- when the victim leaves 30) also need a spell-mod bridge that does not
-- exist. Documented here for when an entry verifies.

local MOB = {
    ["npc_giant_infernal"] = 17908,
    ["npc_abomination"]    = 17898,
    ["npc_ghoul"]          = 17895,
    ["npc_necromancer"]    = 17899,
    ["npc_banshee"]        = 17905,
    ["npc_crypt_fiend"]    = 17897,
    ["npc_gargoyle"]       = 17906,
    ["npc_frost_wyrm"]     = 17907,
    ["npc_fel_stalker"]    = 17916,
}

-- C++-verbatim spell tables. Fields: spell, lo/hi init range,
-- rlo/rhi repeat range (default lo/hi), target "self"/"victim".
-- once: cast once at engage (C++ first-combat-tick flag).
-- refresh: re-cast every 1000ms only while the aura is missing
--   (C++ per-tick HasAura guard, no timer).
-- gate: cast only when the victim's distance passes the gate
--   ("far": dist > d, frost wyrm 25 / "near": dist <= d, gargoyle 20);
--   otherwise the timer stays expired — 1s retry (batrider precedent).
local SPELLS = {
    ["npc_giant_infernal"] = {
        { spell = 31724, lo = 2000, hi = 2000, rlo = 7000, rhi = 7000,
          target = "victim" },
        { spell = 37059, once = true, target = "self" },
    },
    ["npc_abomination"] = {
        { spell = 31607, refresh = true, target = "self" },
        { spell = 31610, lo = 10000, hi = 10000, rlo = 15000, rhi = 25000,
          target = "victim" },
    },
    ["npc_ghoul"] = {
        { spell = 31540, lo = 5000, hi = 10000, rlo = 15000, rhi = 30000,
          target = "self" },
    },
    ["npc_necromancer"] = {
        { spell = 31627, lo = 1000, hi = 6000, rlo = 20000, rhi = 30000,
          target = "victim" },
    },
    ["npc_banshee"] = {
        { spell = 31651, lo = 20000, hi = 25000, target = "victim" },
        { spell = 38183, lo = 15000, hi = 20000, target = "victim" },
        { spell = 31662, lo = 50000, hi = 60000, target = "self" },
    },
    ["npc_crypt_fiend"] = {
        { spell = 28991, lo = 20000, hi = 25000, target = "victim" },
    },
    ["npc_fel_stalker"] = {
        { spell = 31729, lo = 9000, hi = 14000, target = "victim" },
    },
    ["npc_frost_wyrm"] = {
        { spell = 31688, lo = 5000, hi = 5000, rlo = 4000, rhi = 4000,
          target = "victim", gate = { dist = 25, mode = "far" } },
    },
    ["npc_gargoyle"] = {
        { spell = 31664, lo = 2000, hi = 7000, rlo = 2000, rhi = 3000,
          target = "victim", gate = { dist = 20, mode = "near" } },
    },
}

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function resolveTarget(creature, spec)
    if spec.target == "self" then
        return creature
    end
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        return victim
    end
    return nil
end

local function gateOk(creature, target, gate)
    if not gate then
        return true
    end
    local d = creature:GetDistance(target)
    if gate.mode == "far" then
        return d > gate.dist
    end
    return d <= gate.dist
end

local function repeatRange(spec)
    return spec.rlo or spec.lo, spec.rhi or spec.hi
end

local function onSpell(creature, guid, name, idx)
    local spec = SPELLS[name][idx]
    local target = resolveTarget(creature, spec)
    if target and gateOk(creature, target, spec.gate) then
        creature:CastSpell(target, spec.spell)
        local rlo, rhi = repeatRange(spec)
        schedule(guid, "spell" .. idx, math.random(rlo, rhi), function()
            onSpell(creature, guid, name, idx)
        end)
    else
        schedule(guid, "spell" .. idx, 1000, function()
            onSpell(creature, guid, name, idx)
        end)
    end
end

local function onRefresh(creature, guid, name, idx)
    local spec = SPELLS[name][idx]
    if not creature:HasAura(spec.spell) then
        creature:CastSpell(creature, spec.spell)
    end
    schedule(guid, "spell" .. idx, 1000, function()
        onRefresh(creature, guid, name, idx)
    end)
end

local function onEnterCombat(name)
    return function(event, creature, target)
        local guid = creature:GetGUID()
        cancelTimers(guid)
        for i, spec in ipairs(SPELLS[name]) do
            local n, id = name, i
            if spec.once then
                local t = resolveTarget(creature, spec)
                if t then
                    creature:CastSpell(t, spec.spell)
                end
            elseif spec.refresh then
                schedule(guid, "spell" .. i, 0, function()
                    onRefresh(creature, guid, n, id)
                end)
            else
                schedule(guid, "spell" .. i, math.random(spec.lo, spec.hi),
                    function()
                        onSpell(creature, guid, n, id)
                    end)
            end
        end
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

for name, entry in pairs(MOB) do
    RegisterCreatureEvent(entry, 1, onEnterCombat(name))
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    RegisterCreatureEvent(entry, 23, onReset)
end
