-- Garr (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_garr.cpp (boss_garr AI only); molten_core.h:33
-- (BOSS_GARR = 3, fourth boss), :57 (NPC_GARR = 12057).
-- Creature entry: 12057 Garr (C++ ScriptName "boss_garr" per
-- AddSC_boss_garr). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- antimagic pulse 19492 25s then {10s,15s}, non-triggered self-cast
-- (C++ DoCast default is triggered=false — vaelastrasz convention) /
-- magma shackles 19496 15s then {8s,12s}, non-triggered self-cast.
-- OnDied(4)/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Cast gating PORTED (mal_ganis idiom): the C++ UpdateAI
-- UNIT_STATE_CASTING queue gate (boss_garr.cpp:67) and post-event
-- check (:86) are modeled on both timer ticks via the live
-- creature:IsCasting() bridge (engine/world/lua_creature_events.go:1950,
-- CastingUntil window) — a casting tick defers 500ms under its own
-- key, consuming no re-arm. No instance-script model — boss admission
-- via the luaBossAI shim; the _Reset / _JustDied / _JustEngagedWith
-- bookkeeping arms ARE ported below (creature:SetBossState, bridge live
-- lua_creature_events.go:5917): ev1 IN_PROGRESS (JustEngagedWith
-- :53-55 calls the base first), ev23 NOT_STARTED (no Reset override on
-- the boss — base runs), ev4 DONE (no JustDied override — base runs by
-- virtual dispatch); the npc_firesworn DamageTaken arm (eruption +
-- despawn below 10% health) IS ported below — entry 12099 verified
-- from the world DB (TDB creature_template row 12099, name 'Firesworn',
-- ScriptName 'npc_firesworn'; the C++ file carries no entry constant and
-- AddSC_boss_garr only registers the AI class, so the prior
-- "entry-unverifiable" note is retracted). The firesworn immolate
-- schedule (4s then {5s,10s} on a random target) and separation-anxiety
-- check (vs NPC_GARR 20 yd) are NOT ported here — separate arms, not
-- this row. The eruption damage-zeroing arm rides the ev9 pre-damage
-- hook (fireCreatureDamageTaken rewrites the second return) plus
-- creature:Despawn() (bridged, lua_creature_events.go:3305).

local ENTRY_GARR = 12057
local ENTRY_FIRESWORN = 12099

local SPELL_ANTIMAGIC_PULSE = 19492
local SPELL_MAGMA_SHACKLES = 19496
local SPELL_ERUPTION = 19497

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

-- C++ EVENT_ANTIMAGIC_PULSE: non-triggered DoCast(me, 19492);
-- re-arm {10s,15s}.
local function onAntimagicPulse(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "antimagicpulse", 500, function()
            onAntimagicPulse(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ANTIMAGIC_PULSE)
    schedule(guid, "antimagicpulse", math.random(10000, 15000), function()
        onAntimagicPulse(creature, guid)
    end)
end

-- C++ EVENT_MAGMA_SHACKLES: non-triggered DoCast(me, 19496);
-- re-arm {8s,12s}.
local function onMagmaShackles(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "magmashackles", 500, function()
            onMagmaShackles(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_MAGMA_SHACKLES)
    schedule(guid, "magmashackles", math.random(8000, 12000), function()
        onMagmaShackles(creature, guid)
    end)
end

local function garrEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    -- C++ JustEngagedWith (boss_garr.cpp:53-55) calls
    -- BossAI::JustEngagedWith first; _JustEngagedWith writes
    -- SetBossState(BOSS_GARR 3, IN_PROGRESS) (ScriptedCreature.cpp:537).
    creature:SetBossState(3, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "antimagicpulse", 25000, function()
        onAntimagicPulse(creature, guid)
    end)
    schedule(guid, "magmashackles", 15000, function()
        onMagmaShackles(creature, guid)
    end)
end

local function garrResetState(guid)
    cancelTimers(guid)
end

local function garrLeaveCombat(event, creature)
    garrResetState(creature:GetGUIDLow())
end

local function garrDied(event, creature, killer)
    -- No JustDied override in C++ — base BossAI::JustDied runs by
    -- virtual dispatch (ScriptedCreature.h:330); _JustDied() writes
    -- SetBossState(BOSS_GARR 3, DONE).
    creature:SetBossState(3, 3)
    garrResetState(creature:GetGUIDLow())
end

local function garrReset(event, creature)
    -- No Reset override on the boss in C++ — base BossAI::Reset runs;
    -- _Reset() writes SetBossState(BOSS_GARR 3, NOT_STARTED),
    -- alive-gated and skipped when already DONE
    -- (ScriptedCreature.cpp:499-511); Go fires ev23 on the death path
    -- too, hence the gate. (The npc_firesworn Reset at :122-125 is the
    -- ScriptedAI add's own, not the boss's.)
    if creature:IsAlive() and creature:GetBossState(3) ~= 3 then
        creature:SetBossState(3, 0)
    end
    garrResetState(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_GARR, 1, garrEnterCombat)
RegisterCreatureEvent(ENTRY_GARR, 2, garrLeaveCombat)
RegisterCreatureEvent(ENTRY_GARR, 4, garrDied)
RegisterCreatureEvent(ENTRY_GARR, 23, garrReset)

-- Firesworn (npc_firesworn, C++ ScriptName for 12099): the DamageTaken
-- arm (boss_garr.cpp:132-141) is the add's death-by-eruption rule — if
-- the incoming hit would drop it below 10% of max health, the damage is
-- zeroed, it casts eruption 19497 on its victim, and it despawns.
-- C++-exact gate: health10pct = CountPctFromMaxHealth(10) =
-- floor(maxHealth * 10 / 100); the hit triggers when
-- (int32)health - (int32)damage < (int32)health10pct, i.e. strictly
-- below 10% post-damage (a lethal hit also triggers — the subtraction
-- goes negative). The ev9 hook fires pre-application and the second
-- return rewrites the damage (lua_creature_events.go:7548-7589, the
-- julianneDamageTaken precedent: return false, 0); a non-triggering hit
-- passes its damage through unchanged. C++ arm order kept: damage = 0
-- (the rewrite), DoCastVictim (non-triggered 2-arg cast; C++
-- DoCastVictim no-ops with no victim, hence the nil guard), then
-- DespawnOrUnsummon (creature:Despawn, lua_creature_events.go:3305).
-- GetHealth/GetMaxHealth ride the object.go:203 field fallback.
local function fireswornDamageTaken(event, creature, attacker, damage)
    local health10pct = math.floor(creature:GetMaxHealth() * 10 / 100)
    if creature:GetHealth() - damage < health10pct then
        local victim = creature:GetVictim()
        if victim ~= nil then
            creature:CastSpell(victim, SPELL_ERUPTION)
        end
        creature:Despawn()
        return false, 0
    end
    return false, damage
end

RegisterCreatureEvent(ENTRY_FIRESWORN, 9, fireswornDamageTaken)
