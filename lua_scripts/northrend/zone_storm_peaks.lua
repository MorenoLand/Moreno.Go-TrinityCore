-- Storm Peaks zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_storm_peaks.cpp.
--
-- PORTED (SpellScript, RegisterSpellEvent ev5 = OnEffectHitTarget):
--   spell_eject_passenger_wild_wyrm (60603, zone_storm_peaks.cpp:1070-1100):
--   EFFECT_0 SCRIPT_EFFECT. OnEffectHitTarget the hit unit loses
--   SPELL_FIGHT_WYRM (56673). Cast by the wild wyrm on ACTION_GRIP_LOST.
--
-- PORTED (grip cluster, RegisterSpellEvent ev5 = OnEffectHitTarget /
-- RegisterAuraEvent ev1 = OnPeriodic, ev2 = OnApply, ev3 = OnRemove):
--   spell_grip (56689, zone_storm_peaks.cpp:1102-1170): PERIODIC_DUMMY
--   eff0. DummyTick stack-decay schedule (1/tick for the first 15 ticks,
--   then 2*sequence (+1 past the 7th tick of each 15-tick block),
--   :1112-1142) + warn latch below 15 stacks (DoAction ACTION_GRIP_FAILING,
--   :1135-1141) + HandleDrop on EXPIRE removal (DoAction ACTION_GRIP_LOST,
--   :1144-1151). The C++ _tickNumber/_warning members live in a
--   module table keyed by the wyrm GUIDLow, reset on apply — the
--   AuraEffect tick number resets on re-stack, so aura:GetTickNumber()
--   would diverge after a Grab On (zone_storm_peaks.cpp:1160-1162).
--   spell_grab_on (60533, :1171-1198) + spell_loosen_grip (56690 /
--   60586, :1199-1229): DROPPED 2026-11-12 — the ev5 handlers read
--   the caster's 56689 aura, but the player-path spell object
--   returns the player as caster (lua_spell.go:192) while the grip
--   aura lives on the wyrm only (self-cast :835), so the legs could
--   never fire; Go owns both arms via modWyrmGripStacks (spells.go,
--   the 63975 precedent).
--   spell_low_health_trigger (60596, :1230-1261): EFFECT_0
--   SCRIPT_EFFECT. OnEffectHitTarget the hit unit casts GetEffectValue()
--   (the DBC eff0 spell, validated :1242) on a nil target, triggered
--   (:1247); the port casts it on the hit unit (self-cast), the
--   close-rift precedent — Go CastSpell(nil, ...) falls back to the
--   current victim.
--   spell_claw_swipe_check (56705, :1290-1347): EFFECT_0 DUMMY.
--   AfterEffectApply warns (DoAction ACTION_CLAW_SWIPE_WARN, :1299-1302);
--   AfterEffectRemove dodges when the seat-0 passenger holds
--   SPELL_DODGE_CLAWS (56704), else the target casts the eff0 amount
--   (60776) on a nil target, non-triggered (:1304-1319); the port casts
--   it on the target (self-cast, same precedent).
--   DORMANT: the wild-wyrm AI (npc_wild_wyrm, :702-1090, VehicleAI +
--   scheduler + movement + phase machine) is unported, so nothing casts
--   grip / grab-on / thrust-spear / low-health-trigger / claw-swipe yet,
--   and the DoAction(1..4) call sites fire Lua OnDoAction hooks with no
--   consumer until that AI ports; the spell arms are also DORMANT until
--   Spell.dbc ships the records.
--   spell_falling_dragon_feign_death (55795, :1376-1408): EFFECT_0
--   DUMMY. Apply sets UNIT_FLAG_UNK_29 + UNIT_FLAG2_FEIGN_DEATH on the
--   wyrm; remove clears both. Cast by the wyrm on itself at dead-phase
--   (:814); the dead-phase machine itself (DamageTaken latch, MoveFall)
--   is npc_wild_wyrm AI, unported.
--   spell_player_mount_wyrm (56672, :1411-1445): EFFECT_0 DUMMY. On
--   remove the player casts SPELL_FIGHT_WYRM (56673) on itself,
--   triggered. Cast by the wyrm on the player at POINT_START_FIGHT
--   (:782). Player-aura ev2/ev3 fire via the spells.go apply/remove
--   funnels (fireAuraApply/fireAuraRemove on the player object).
--
-- PORTED (tick 205, RegisterAuraEvent OnApply = 2 / OnPeriodic = 1):
--   spell_jokkum_scriptcast (61319, zone_storm_peaks.cpp:967-996):
--   EFFECT_0 DUMMY. OnApply the target casts Jokkum Summon (56541) on
--   itself, triggered. Quest 13010 "Krolmir, Hammer of Storms".
--   Aura binding is DB-side only: TDB spell_script_names row
--   (61319,'spell_jokkum_scriptcast')
--   (sql/old/3.3.5a/TDB52_to_TDB53_updates/world/
--   2013_11_24_00_world_spell_script_names.sql), Wowhead "Jokkum
--   Scriptcast" Effect 0 is Apply Aura: Dummy, 500 ms.
--   spell_veranus_summon (56650, zone_storm_peaks.cpp:998-1027):
--   EFFECT_0 DUMMY. OnApply the target casts Summon Veranus and Thorim
--   (56649) on itself, triggered. Quest "Territorial Trespass".
--   Aura binding is DB-side only: TDB spell_script_names row
--   (56650,'spell_veranus_summon')
--   (sql/old/3.3.5a/TDB52_to_TDB53_updates/world/
--   2013_12_23_00_world_misc.sql), Wowhead "Player Cast Veranus
--   Summon" Effect 0 is Apply Aura: Dummy, 500 ms.
--   spell_close_rift (56763, zone_storm_peaks.cpp:1034-1065):
--   EFFECT_0 PERIODIC_TRIGGER_SPELL. On the 5th periodic tick the
--   target casts Despawn Rift (61665) with a nil target, triggered.
--   Aura binding is DB-side only: TDB spell_script_names row
--   (56763,'spell_close_rift')
--   (sql/old/3.3.5a/TDB49_to_TDB50_updates/world/
--   2012_10_21_01_world_misc.sql), Wowhead "Close Rift" Effect 0 is
--   Apply Aura: Periodically trigger spell, every 1 second, 5 sec.
-- DELTA (close rift): C++ casts with a nullptr target; Go
-- creature:CastSpell(nil, ...) falls back to the target's current
-- victim and no-ops without one, so the port casts explicitly on the
-- target (self-cast, per the tick-193 triage note). The port returns
-- nothing, so the Go default trigger cast still fires each tick, as
-- in C++ (no PreventDefaultAction in the handler).

local SPELL_JOKKUM_SCRIPTCAST = 61319 -- TDB spell_script_names
local SPELL_JOKKUM_SUMMON = 56541 -- zone_storm_peaks.cpp:952
local SPELL_PLAYER_CAST_VERANUS_SUMMON = 56650 -- TDB spell_script_names
local SPELL_SUMMON_VERANUS_AND_THORIM = 56649 -- zone_storm_peaks.cpp:955
local SPELL_CLOSE_RIFT = 56763 -- TDB spell_script_names
local SPELL_DESPAWN_RIFT = 61665 -- zone_storm_peaks.cpp:1031
local SPELL_PLAYER_MOUNT_WYRM = 56672 -- zone_storm_peaks.cpp:633
local SPELL_FIGHT_WYRM = 56673 -- zone_storm_peaks.cpp:634
local SPELL_EJECT_PASSENGER_1 = 60603 -- zone_storm_peaks.cpp:647
local SPELL_GRIP = 56689 -- zone_storm_peaks.cpp:636
local SPELL_LOW_HEALTH_TRIGGER = 60596 -- zone_storm_peaks.cpp:644
local SPELL_DODGE_CLAWS = 56704 -- zone_storm_peaks.cpp:638
local SPELL_CLAW_SWIPE_CHECK = 56705 -- zone_storm_peaks.cpp:1290
local SPELL_FALLING_DRAGON_FEIGN_DEATH = 55795 -- zone_storm_peaks.cpp:656
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h
local UNIT_FIELD_FLAGS_2 = 60 -- UpdateFields.h:118 (OBJECT_END+0x36)
local UNIT_FLAG_UNK_29 = 0x20000000 -- UnitDefines.h:153
local UNIT_FLAG2_FEIGN_DEATH = 0x00000001 -- UnitDefines.h:161
local ACTION_CLAW_SWIPE_WARN = 1 -- zone_storm_peaks.cpp:665
local ACTION_CLAW_SWIPE_DODGE = 2 -- zone_storm_peaks.cpp:666
local ACTION_GRIP_FAILING = 3 -- zone_storm_peaks.cpp:667
local ACTION_GRIP_LOST = 4 -- zone_storm_peaks.cpp:668
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:61-69

-- C++ spell_jokkum_scriptcast_AuraScript::OnApply
-- (zone_storm_peaks.cpp:980-984).
local function jokkumScriptcastApply(event, aura, target)
    target:CastSpell(target, SPELL_JOKKUM_SUMMON, true)
end

RegisterAuraEvent(SPELL_JOKKUM_SCRIPTCAST, 2, jokkumScriptcastApply)

-- C++ spell_veranus_summon_AuraScript::OnApply
-- (zone_storm_peaks.cpp:1011-1015).
local function veranusSummonApply(event, aura, target)
    target:CastSpell(target, SPELL_SUMMON_VERANUS_AND_THORIM, true)
end

RegisterAuraEvent(SPELL_PLAYER_CAST_VERANUS_SUMMON, 2, veranusSummonApply)

-- C++ spell_close_rift_AuraScript::HandlePeriodic
-- (zone_storm_peaks.cpp:1048-1052): _counter++ == 5 on the 5th tick
-- is aura:GetTickNumber() == 5 (first tick reads 1, tick-191 bridge).
local function closeRiftPeriodic(event, aura, target)
    if aura:GetTickNumber() == 5 then
        target:CastSpell(target, SPELL_DESPAWN_RIFT, true)
    end
end

RegisterAuraEvent(SPELL_CLOSE_RIFT, 1, closeRiftPeriodic)

-- C++ spell_eject_passenger_wild_wyrm::HandleScript
-- (zone_storm_peaks.cpp:1084-1087): EFFECT_0 SCRIPT_EFFECT
-- OnEffectHitTarget — the hit unit loses SPELL_FIGHT_WYRM (56673).
-- Cast by the wild wyrm on ACTION_GRIP_LOST (DoCastAOE, :746); the
-- hit units are the passengers. Fires on the creature path only (ev5,
-- runtime.go:574-577); the return value is ignored. DORMANT until
-- Spell.dbc ships the 60603 record.
local function ejectPassengerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAura(SPELL_FIGHT_WYRM)
end

RegisterSpellEvent(SPELL_EJECT_PASSENGER_1, 5, ejectPassengerHit)

-- Grip per-aura state: the C++ spell_grip_AuraScript members _tickNumber
-- and _warning (zone_storm_peaks.cpp:1158-1160), keyed by the wyrm
-- GUIDLow. A Lua handler is stateless, so the table carries them; the
-- entry is created on apply and dropped on remove / expiry.
local gripState = {}

-- C++ spell_grip_AuraScript members (implicit: _tickNumber = 0,
-- _warning = false at script-instance creation, :1160-1162).
local function gripApply(event, aura, target)
    if target == nil then
        return
    end
    gripState[target:GetGUIDLow()] = { ticks = 0, warned = false }
end

RegisterAuraEvent(SPELL_GRIP, 2, gripApply)

-- C++ spell_grip_AuraScript::DummyTick (zone_storm_peaks.cpp:1112-1142):
-- PERIODIC_DUMMY eff0. Each 15 ticks the stack reduction grows by 2
-- (1 at each 7th and 15th tick past the first block); the first 15 ticks
-- remove 1 stack each. Below 15 stacks the wyrm warns once
-- (ACTION_GRIP_FAILING); above 30 the warn latch resets.
-- DELTA: C++ removes the aura mid-loop via ModStackAmount(-1,
-- AURA_REMOVE_BY_EXPIRE) when a decrement hits 0, which fires HandleDrop;
-- Go SetStackAmount(0) does not remove (50240 precedent), so the port
-- removes explicitly with mode 4 (EXPIRE) once the count would hit 0 —
-- the ev3 handler below then fires the drop arm. The warn check never
-- runs on the expiry tick; in C++ it could fire there only when unwarned,
-- which cannot happen (the count passes 14 on the way down).
local function gripPeriodic(event, aura, target)
    if target == nil then
        return
    end
    local key = target:GetGUIDLow()
    local state = gripState[key]
    if state == nil then
        state = { ticks = 0, warned = false }
        gripState[key] = state
    end
    state.ticks = state.ticks + 1
    local period = ((state.ticks - 1) % 15) + 1
    local sequence = math.floor((state.ticks - 1) / 15)
    local stacksToRemove
    if sequence == 0 then
        stacksToRemove = 1
    else
        stacksToRemove = sequence * 2
        if period > 7 then
            stacksToRemove = stacksToRemove + 1
        end
    end
    local stacks = aura:GetStackAmount()
    if stacks <= stacksToRemove then
        gripState[key] = nil
        aura:Remove(AURA_REMOVE_BY_EXPIRE)
        return
    end
    local newStacks = stacks - stacksToRemove
    aura:SetStackAmount(newStacks)
    if newStacks < 15 and not state.warned then
        state.warned = true
        target:DoAction(ACTION_GRIP_FAILING)
    elseif newStacks > 30 then
        state.warned = false
    end
end

RegisterAuraEvent(SPELL_GRIP, 1, gripPeriodic)

-- C++ spell_grip_AuraScript::HandleDrop (zone_storm_peaks.cpp:1144-1151):
-- AfterEffectRemove eff0, REAL handle. Only an EXPIRE removal drops the
-- grip (the phase-2 RemoveAurasDueToSpell(SPELL_GRIP) uses DEFAULT and
-- must not eject); the wyrm ejects the passenger and evades
-- (ACTION_GRIP_LOST, npc_wild_wyrmAI::DoAction :745-747).
local function gripRemove(event, aura, target)
    if target == nil or aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    gripState[target:GetGUIDLow()] = nil
    target:DoAction(ACTION_GRIP_LOST)
end

RegisterAuraEvent(SPELL_GRIP, 3, gripRemove)

-- C++ spell_low_health_trigger_SpellScript::HandleScript
-- (zone_storm_peaks.cpp:1247): EFFECT_0 SCRIPT_EFFECT
-- OnEffectHitTarget — the hit unit casts GetEffectValue() (the DBC eff0
-- spell) on a nil target, triggered. The port casts it on the hit unit
-- (self-cast, the close-rift precedent); a 0 value (no DBC row) is a
-- no-op. Cast by the wyrm at 25% health (DoCastAOE, :849-851) for the
-- phase-2 transition. DORMANT until Spell.dbc ships 60596.
local function lowHealthTriggerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value <= 0 then
        return
    end
    hitTarget:CastSpell(hitTarget, value, true)
end

RegisterSpellEvent(SPELL_LOW_HEALTH_TRIGGER, 5, lowHealthTriggerHit)

-- C++ spell_claw_swipe_check_AuraScript::OnApply
-- (zone_storm_peaks.cpp:1299-1302): AfterEffectApply eff0 DUMMY — the
-- wyrm warns (ACTION_CLAW_SWIPE_WARN, Talk SAY_SWIPE, :736-738).
local function clawSwipeCheckApply(event, aura, target)
    if target == nil then
        return
    end
    target:DoAction(ACTION_CLAW_SWIPE_WARN)
end

RegisterAuraEvent(SPELL_CLAW_SWIPE_CHECK, 2, clawSwipeCheckApply)

-- C++ spell_claw_swipe_check_AuraScript::OnRemove
-- (zone_storm_peaks.cpp:1304-1319): AfterEffectRemove eff0 DUMMY. When
-- the seat-0 passenger holds SPELL_DODGE_CLAWS (56704) the swipe is
-- dodged (ACTION_CLAW_SWIPE_DODGE); otherwise the target casts the eff0
-- amount (60776 Claw Swipe) on a nil target, non-triggered. The port
-- casts it on the target (self-cast, the close-rift precedent); a 0
-- amount (no DBC row) is a no-op. DORMANT until Spell.dbc ships 56705.
local function clawSwipeCheckRemove(event, aura, target)
    if target == nil then
        return
    end
    local kit = target:GetVehicleKit()
    if kit ~= nil then
        local passenger = kit:GetPassenger(0)
        if passenger ~= nil and passenger:HasAura(SPELL_DODGE_CLAWS) then
            target:DoAction(ACTION_CLAW_SWIPE_DODGE)
            return
        end
    end
    local amount = aura:GetAmount(0)
    if amount <= 0 then
        return
    end
    target:CastSpell(target, amount, false)
end

RegisterAuraEvent(SPELL_CLAW_SWIPE_CHECK, 3, clawSwipeCheckRemove)

-- C++ spell_falling_dragon_feign_death_AuraScript
-- (zone_storm_peaks.cpp:1386-1408): EFFECT_0 DUMMY, REAL handle.
-- AfterEffectApply sets UNIT_FLAG_UNK_29 (UNIT_FIELD_FLAGS) +
-- UNIT_FLAG2_FEIGN_DEATH (UNIT_FIELD_FLAGS_2) on the wyrm;
-- AfterEffectRemove clears both. Cast by the wyrm on itself
-- (DoCastAOE) when the dead-phase starts (:814); the dead-phase
-- machine (DamageTaken latch, MoveFall, KillSelf) is npc_wild_wyrm
-- AI, unported, so nothing casts this yet. DORMANT until Spell.dbc
-- ships the 55795 record.
local function feignDeathApply(event, aura, target)
    if target == nil then
        return
    end
    target:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_UNK_29)
    target:SetFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_FEIGN_DEATH)
end

RegisterAuraEvent(SPELL_FALLING_DRAGON_FEIGN_DEATH, 2, feignDeathApply)

local function feignDeathRemove(event, aura, target)
    if target == nil then
        return
    end
    target:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_UNK_29)
    target:RemoveFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_FEIGN_DEATH)
end

RegisterAuraEvent(SPELL_FALLING_DRAGON_FEIGN_DEATH, 3, feignDeathRemove)

-- C++ spell_player_mount_wyrm_AuraScript
-- (zone_storm_peaks.cpp:1426-1430): EFFECT_0 DUMMY, REAL handle.
-- AfterEffectRemove the target (the player) casts SPELL_FIGHT_WYRM
-- (56673) on a nil target, triggered. Cast by the wyrm on the player
-- at POINT_START_FIGHT (:782). The port casts it on the target
-- (self-cast, the close-rift precedent); Validate (56673 in DBC) has
-- no Lua bridge. DORMANT until Spell.dbc ships the 56672 record.
local function mountWyrmRemove(event, aura, target)
    if target == nil then
        return
    end
    target:CastSpell(target, SPELL_FIGHT_WYRM, true)
end

RegisterAuraEvent(SPELL_PLAYER_MOUNT_WYRM, 3, mountWyrmRemove)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12823_remove_collapsing_cave_aura::HandleScriptEffect
--   (zone_storm_peaks.cpp:1449-1468): EFFECT_0 SCRIPT_EFFECT. The
--   hit unit loses the Collapsing Cave aura (55486; RemoveAura is
--   the RemoveAurasDueToSpell twin, lua_player.go:4019 /
--   lua_creature_events.go:1676). Binding 55693 (AC
--   'spell_q12823_remove_collapsing_cave_aura'; TDB 335.26091
--   renames it 'spell_gen_cancel_aura').
local SPELL_REMOVE_COLLAPSING_CAVE_AURA = 55693 -- zone_storm_peaks.cpp:1449
local SPELL_COLLAPSING_CAVE = 55486 -- zone_storm_peaks.cpp:1445

local function removeCollapsingCaveAuraHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    hitTarget:RemoveAura(SPELL_COLLAPSING_CAVE)
end

RegisterSpellEvent(SPELL_REMOVE_COLLAPSING_CAVE_AURA, 5, removeCollapsingCaveAuraHit)
