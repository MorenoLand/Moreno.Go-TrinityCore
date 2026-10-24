-- Imprisoned Beryl Sorcerer (Borean Tundra, quest 11648) --
-- Lua port of src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (npc_imprisoned_beryl_sorcererAI — rebuff timer + SpellHit impact arms).
-- Zone-script unit per northrend_script_loader.cpp order (decl 199 /
-- call 394; dalaran done, borean_tundra: 20 registrations).
-- Retraction re-audit of the 15:34 "CLOSED document-only" entry: that
-- audit dismissed the whole file under stale "no-timer" claims. The two
-- arms below are ungated and bridgeable.
-- Entry: NPC_IMPRISONED_BERYL_SORCERER = 25478 (ImprisionedBerylSorcerer
-- enum :1180, C++-verifiable).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 14 OnHitBySpell,
-- 4 OnDied (no OnLeaveCombat: the C++ timer runs out of combat).
-- Ported arm 1 (C++ UpdateAI rebuff raw timer :1212-1221): rebuff inits
-- 0 (Initialize :1196, Reset :1201 calls Initialize) so the first fire
-- is immediate on spawn/reset, then every 180000ms; the UpdateAI tick
-- is ungated (UpdateVictim() result discarded — the timer runs out of
-- combat too, so there is NO LeaveCombat cancel; death (event 4)
-- cancels because C++ UpdateAI stops on death).
-- if (!me->HasAura(SPELL_COSMETIC_ENSLAVE_CHAINS_SELF 45631, :1090))
-- DoCast(me, 45631) — 2-arg non-triggered self-cast; the HasAura gate
-- is the bridged creature:HasAura (gunship precedent).
-- Ported arm 2 (C++ SpellHit :1232-1240 -> GotStinged :1242-1261):
-- player-caster gate (caster->ToPlayer(), :1234) via the
-- caster:GetObjectType()=="Player" convention (nalorakk precedent);
-- the event-14 receive-side bridge is engine-fired for player casts
-- (spells.go) — the "no-SpellHit bridge" doctrine is stale. spellId
-- 45634 SPELL_NEURAL_NEEDLE -> DoCastSelf(45702
-- SPELL_NEURAL_NEEDLE_IMPACT); spellId 48252
-- SPELL_PROTOTYPE_NEURAL_NEEDLE -> DoCastSelf(48254
-- SPELL_PROTOTYPE_NEURAL_NEEDLE_IMPACT) — DoCastSelf is the 2-arg
-- non-triggered self-cast, unconditional in GotStinged (fires before
-- the quest gate :1247).
-- Unmodeled: GotStinged quest-11648 status gate + step computation
-- (caster->GetAuraCount(45634) / GetAuraCount(48252)+7, :1250-1255)
-- + Talk(step, caster) + KilledMonsterCredit(25478) (:1260/:1258) —
-- no quest-status / aura-count / kill-credit bridges; the JustEngagedWith
-- empty override; Reset's REACT_PASSIVE leg (zone_borean_tundra.cpp
-- :1204-:1209) IS ported via berylSorcererSpawnOrReset below — the C++
-- != REACT_PASSIVE gate needs no GetReactState bridge: setting PASSIVE
-- unconditionally is C++-exact (a set when already passive is a no-op),
-- so the old "no GetReactState bridge" note is moot, not a deviation.
-- Go: RegisterLuaBoss("npc_imprisoned_beryl_sorcerer", 25478) — the
-- no-op AI neutralizes the default combat behavior for this passive
-- NPC (storm_cloud / blessed_banner zone-script precedent).

local ENTRY_IMPRISONED_BERYL_SORCERER = 25478

local SPELL_COSMETIC_ENSLAVE_CHAINS_SELF = 45631
local SPELL_NEURAL_NEEDLE = 45634
local SPELL_PROTOTYPE_NEURAL_NEEDLE = 48252
local SPELL_NEURAL_NEEDLE_IMPACT = 45702
local SPELL_PROTOTYPE_NEURAL_NEEDLE_IMPACT = 48254

local REBUFF_INTERVAL = 180000

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

-- C++ :1212-1220 rebuff arm: !HasAura(45631) -> DoCast(me, 45631)
-- non-triggered; re-arm 180000ms.
local function onRebuff(creature, guid)
    if not creature:HasAura(SPELL_COSMETIC_ENSLAVE_CHAINS_SELF) then
        creature:CastSpell(creature, SPELL_COSMETIC_ENSLAVE_CHAINS_SELF)
    end
    schedule(guid, "rebuff", REBUFF_INTERVAL, function() onRebuff(creature, guid) end)
end

-- C++ Reset()/spawn: rebuff inits 0 -> immediate first fire, then the
-- 180s loop (storm_cloud OnSpawn(5)/OnReset(23) convention).
local function berylSorcererSpawnOrReset(event, creature)
    -- Event 23 fires ahead of event 4 on the kill path (fireCreatureDied);
    -- C++ Reset() re-inits the rebuff timer on a live creature only, so a
    -- dead-state 23 fire must not cast or re-arm.
    if event == 23 and creature:IsDead() then
        return
    end
    -- C++ npc_imprisoned_beryl_sorcererAI::Reset
    -- (zone_borean_tundra.cpp :1204-:1209): SetReactState(REACT_PASSIVE)
    -- (the != REACT_PASSIVE gate has no bridge — the set itself is a
    -- no-op when already passive); arm order matches C++ (passive leg
    -- before the rebuff Initialize).
    creature:SetReactState(0) -- REACT_PASSIVE
    local guid = creature:GetGUID()
    cancelTimers(guid)
    onRebuff(creature, guid)
end

-- C++ UpdateAI (zone_borean_tundra.cpp:1212-1221) discards
-- UpdateVictim()'s result, so the rebuff timer deliberately ticks in
-- and out of combat — no LeaveCombat cancel. Death stops the C++
-- UpdateAI, so the timer dies with the creature (event 4 only).
local function berylSorcererDied(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ SpellHit :1232-1240 -> GotStinged :1242-1244: player-caster gate,
-- then the unconditional DoCastSelf impact leg per spell id.
local function berylSorcererSpellHit(event, creature, caster, spellId)
    if not caster or caster:GetObjectType() ~= "Player" then
        return
    end
    if spellId == SPELL_NEURAL_NEEDLE then
        creature:CastSpell(creature, SPELL_NEURAL_NEEDLE_IMPACT)
    elseif spellId == SPELL_PROTOTYPE_NEURAL_NEEDLE then
        creature:CastSpell(creature, SPELL_PROTOTYPE_NEURAL_NEEDLE_IMPACT)
    end
end

RegisterCreatureEvent(ENTRY_IMPRISONED_BERYL_SORCERER, 5, berylSorcererSpawnOrReset)
RegisterCreatureEvent(ENTRY_IMPRISONED_BERYL_SORCERER, 23, berylSorcererSpawnOrReset)
RegisterCreatureEvent(ENTRY_IMPRISONED_BERYL_SORCERER, 14, berylSorcererSpellHit)
RegisterCreatureEvent(ENTRY_IMPRISONED_BERYL_SORCERER, 4, berylSorcererDied)
