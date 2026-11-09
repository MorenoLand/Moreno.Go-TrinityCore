-- Krik'thir the Gatewatcher (Azjol-Nerub) — Lua port of the AuraScript
-- arm in src/server/scripts/Northrend/AzjolNerub/AzjolNerub/boss_krikthir_the_gatewatcher.cpp.
--
-- PORTED (tick 198, RegisterAuraEvent OnRemove = 3):
--   spell_gatewatcher_web_wrap::HandleEffectRemove
--   (boss_krikthir_the_gatewatcher.cpp:1015-1023): EFFECT_0
--   SPELL_AURA_MOD_ROOT. On removal by expiry only, the target casts
--   Web Wrap Wrapped 52087 on itself, triggered. The mode gate is
--   Aura:GetRemoveMode (new tick 198: SpellAuraDefines.h:61-69 values).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the wrapped player's
-- triggered self-cast lands non-triggered; pre-existing bridge gap,
-- same as all prior player-target ports.
--
-- achievement_watch_him_die — PORTED tick 417: the C++ OnCheck
-- (boss_krikthir_the_gatewatcher.cpp:1036-1061) requires the three
-- Watchers (Gashra 28730 / Narjil 28729 / Silthik 28731) all ALIVE at
-- Krik'thir kill time — the loop `continue`s only while each watcher
-- is alive and returns false on the first dead/missing one, so the
-- gate passes only with zero watcher deaths. The Go gate therefore
-- reads a Lua-maintained death counter on Krik'thir's motion and
-- passes when it is 0 (no Go-side instance-creature-alive reader):
-- each watcher's ev4 (watcherDied below, the C++ JustDied ->
-- DoAction relay, :432-434/:510-512/:588-590) increments
-- SetData(DATA_WATCHERS_DEAD); the boss GUID is latched by the ev5
-- SetGuidData (sjonnir precedent; C++ instance_azjol_nerub.cpp:36
-- latches NPC_KRIKTHIR under DATA_KRIKTHIR). DATA_WATCHERS_DEAD is
-- a Lua-side slot — the C++ boss AI's only SetData slot is
-- DATA_PET_GROUP = 0 (:105), so 1 is free. No Reset write: dead
-- watchers stay dead across a wipe (the counter must survive evade),
-- and a fresh motion reads 0, the C++ all-alive default. (Coordinator
-- correction tick 417: the worker's first draft read the loop
-- backwards as "all dead"; the C++ text says all alive.)

local ENTRY_KRIKTHIR = 28684 -- azjol_nerub.h:46
local DATA_KRIKTHIR = 0 -- azjol_nerub.h:31
local DATA_WATCHERS_DEAD = 1 -- Lua-side slot (see header note)
local DATA_WATCHER_NARJIL = 3 -- azjol_nerub.h ANDataTypes
local DATA_WATCHER_GASHRA = 4 -- azjol_nerub.h ANDataTypes
local DATA_WATCHER_SILTHIK = 5 -- azjol_nerub.h ANDataTypes
local DATA_GATEWATCHER_GREET = 8 -- azjol_nerub.h ANDataTypes
local ACTION_GATEWATCHER_GREET = 1 -- azjol_nerub.h InstanceActions
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)
local SAY_PREFIGHT = 4 -- boss_krikthir_the_gatewatcher.cpp:124
local NPC_WATCHER_NARJIL = 28729 -- azjol_nerub.h:50
local NPC_WATCHER_GASHRA = 28730 -- azjol_nerub.h:51
local NPC_WATCHER_SILTHIK = 28731 -- azjol_nerub.h:52

local SPELL_WEB_WRAP = 52086 -- boss_krikthir_the_gatewatcher.cpp:74
local SPELL_WEB_WRAP_WRAPPED = 52087 -- boss_krikthir_the_gatewatcher.cpp:75
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

local function webWrapRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    if target ~= nil then
        target:CastSpell(target, SPELL_WEB_WRAP_WRAPPED, true)
    end
end

RegisterAuraEvent(SPELL_WEB_WRAP, 3, webWrapRemove)

-- C++ boss-krik'thir GUID latching (instance_azjol_nerub.cpp:36:
-- NPC_KRIKTHIR -> DATA_KRIKTHIR) — the Lua mirror is the ev5
-- SetGuidData (sjonnir precedent), so the watcher relay below can
-- resolve the boss.
local function krikthirSpawned(event, creature)
    creature:SetGuidData(DATA_KRIKTHIR)
end

-- C++ npc_watcher_{gashra,narjil,silthik}::JustDied
-- (boss_krikthir_the_gatewatcher.cpp:432-434/:510-512/:588-590): a
-- killed Watcher relays DoAction(ACTION_*_DIED) to Krik'thir via
-- instance GetCreature(DATA_KRIKTHIR). The Lua twin resolves the
-- boss via creature:GetInstanceCreature (illidari-council precedent)
-- and increments the dead-watchers counter on his motion. Event 4
-- fires on the watcher with (event, creature, killer).
local function watcherDied(event, creature, killer)
    local krikthir = creature:GetInstanceCreature(DATA_KRIKTHIR)
    if krikthir then
        krikthir:SetData(DATA_WATCHERS_DEAD, krikthir:GetData(DATA_WATCHERS_DEAD) + 1)
    end
end

-- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
-- instance->SetBossState(DATA_KRIKTHIR, DONE). The instance
-- script's OnUnitDeath gate reads the flag below (no
-- boss-state-change event exists — ahnkahet precedent).
local function krikthirDied(event, creature, killer)
    creature:SetBossState(DATA_KRIKTHIR, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.krikthirDone = true
    end
end

-- C++ boss_krik_thir::DoAction(-ACTION_GATEWATCHER_GREET)
-- (boss_krikthir_the_gatewatcher.cpp:214-219): if the greet flag
-- is unset and the boss is alive, out of combat and its pets are
-- not in combat, set the flag and Talk(SAY_PREFIGHT). The flag
-- write rides instanceData:SetData (instance_azjol_nerub.lua
-- OnSetData stores it); the read is the stored field.
-- DEVIATION: _petsInCombat is boss-AI state with no Lua twin, so
-- the greet can fire while pets are in combat; C++ suppresses it
-- then. The flag still caps it at one Talk.
local function krikthirDoAction(event, creature, action)
    if action ~= -ACTION_GATEWATCHER_GREET then
        return
    end
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data == nil then
        return
    end
    if (data.gatewatcherGreet or 0) ~= 0 then
        return
    end
    if not creature:IsAlive() or creature:IsInCombat() then
        return
    end
    data:SetData(DATA_GATEWATCHER_GREET, 1)
    creature:Talk(SAY_PREFIGHT)
end

-- C++ creatureData latching (instance_azjol_nerub.cpp:36-41):
-- the watcher rows carry the ROTATED mapping (narjil->GASHRA,
-- gashra->SILTHIK, silthik->NARJIL) — ported verbatim. The
-- instance-side OnCreatureCreate route is dead for static spawns
-- (instance_hooks.go:553-562), so the latches ride ev5 here.
local WATCHER_GUID_DATA = {
    [NPC_WATCHER_NARJIL] = DATA_WATCHER_GASHRA,
    [NPC_WATCHER_GASHRA] = DATA_WATCHER_SILTHIK,
    [NPC_WATCHER_SILTHIK] = DATA_WATCHER_NARJIL,
}

local function watcherSpawned(event, creature)
    creature:SetGuidData(WATCHER_GUID_DATA[creature:GetEntry()])
end

RegisterCreatureEvent(ENTRY_KRIKTHIR, 5, krikthirSpawned)
RegisterCreatureEvent(ENTRY_KRIKTHIR, 4, krikthirDied)
RegisterCreatureEvent(ENTRY_KRIKTHIR, 38, krikthirDoAction)
RegisterCreatureEvent(NPC_WATCHER_NARJIL, 4, watcherDied)
RegisterCreatureEvent(NPC_WATCHER_GASHRA, 4, watcherDied)
RegisterCreatureEvent(NPC_WATCHER_SILTHIK, 4, watcherDied)
RegisterCreatureEvent(NPC_WATCHER_NARJIL, 5, watcherSpawned)
RegisterCreatureEvent(NPC_WATCHER_GASHRA, 5, watcherSpawned)
RegisterCreatureEvent(NPC_WATCHER_SILTHIK, 5, watcherSpawned)
