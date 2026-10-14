// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { NewInstallOnboardingModal } from "@/app/onboarding/onboarding";
import { CurrentOnboardingVersion } from "@/app/onboarding/onboarding-common";
import { UpgradeOnboardingModal } from "@/app/onboarding/onboarding-upgrade";
import { onboardingToOpen } from "@/app/onboarding/onboardingpolicy";
import { ClientModel } from "@/app/store/client-model";
import { globalStore } from "@/app/store/jotaiStore";
import { atoms, globalPrimaryTabStartup } from "@/store/global";
import { modalsModel } from "@/store/modalmodel";
import * as jotai from "jotai";
import { useEffect } from "react";
import { getModalComponent } from "./modalregistry";

const ModalsRenderer = () => {
    const clientData = jotai.useAtomValue(ClientModel.getInstance().clientAtom);
    const [newInstallOnboardingOpen, setNewInstallOnboardingOpen] = jotai.useAtom(modalsModel.newInstallOnboardingOpen);
    const [upgradeOnboardingOpen, setUpgradeOnboardingOpen] = jotai.useAtom(modalsModel.upgradeOnboardingOpen);
    const [modals] = jotai.useAtom(modalsModel.modalsAtom);
    const rtn: React.ReactElement[] = [];
    for (const modal of modals) {
        const ModalComponent = getModalComponent(modal.displayName);
        if (ModalComponent) {
            rtn.push(<ModalComponent key={modal.displayName} {...modal.props} />);
        }
    }
    if (newInstallOnboardingOpen) {
        rtn.push(<NewInstallOnboardingModal key={NewInstallOnboardingModal.displayName} />);
    }
    if (upgradeOnboardingOpen) {
        rtn.push(<UpgradeOnboardingModal key={UpgradeOnboardingModal.displayName} />);
    }
    useEffect(() => {
        const choice = onboardingToOpen(
            clientData.tosagreed,
            clientData.meta?.["onboarding:lastversion"],
            CurrentOnboardingVersion,
            globalPrimaryTabStartup
        );
        if (choice === "new") {
            setNewInstallOnboardingOpen(true);
        }
        if (choice === "upgrade") {
            setUpgradeOnboardingOpen(true);
        }
    }, [clientData]);
    useEffect(() => {
        globalStore.set(atoms.modalOpen, rtn.length > 0);
    }, [rtn]);

    return <>{rtn}</>;
};

export { ModalsRenderer };
